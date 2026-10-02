package core_test

// PC-121: each architectural check behind the PCI DSS / SOC 2 catalogs, proven in BOTH directions on
// synthetic Terraform ingested through the real HCL path (golden bundles cannot show a failure for most of
// them). A control must fail when it should, support when it should, and say not_assessable when the model
// cannot know.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

const complianceBundle = `
resource "aws_vpc" "v" { cidr_block = "10.0.0.0/16" }
resource "aws_internet_gateway" "igw" { vpc_id = aws_vpc.v.id }
resource "aws_subnet" "s" {
  vpc_id            = aws_vpc.v.id
  cidr_block        = "10.0.1.0/24"
  availability_zone = "eu-west-1a"
}
resource "aws_eip" "e" { domain = "vpc" }
resource "aws_nat_gateway" "n" {
  allocation_id = aws_eip.e.id
  subnet_id     = aws_subnet.s.id
}
resource "aws_route_table" "rt" {
  vpc_id = aws_vpc.v.id
  %ROUTE%
}
resource "aws_route_table_association" "a" {
  subnet_id      = aws_subnet.s.id
  route_table_id = aws_route_table.rt.id
}
resource "aws_security_group" "db" {
  vpc_id = aws_vpc.v.id
  %SGRULES%
}
resource "aws_db_subnet_group" "g" {
  name       = "g"
  subnet_ids = [aws_subnet.s.id]
}
resource "aws_db_instance" "d" {
  identifier             = "d"
  db_subnet_group_name   = aws_db_subnet_group.g.name
  vpc_security_group_ids = [aws_security_group.db.id]
  %DBATTRS%
}
resource "aws_lb" "front" {
  name    = "front"
  subnets = [aws_subnet.s.id]
  %LBATTRS%
}
%EXTRA%
`

type cfg struct{ route, sgrules, dbattrs, lbattrs, extra string }

func complianceIR(t *testing.T, c cfg) *core.IR {
	t.Helper()
	dir := t.TempDir()
	src := strings.NewReplacer("%ROUTE%", c.route, "%SGRULES%", c.sgrules, "%DBATTRS%", c.dbattrs, "%LBATTRS%", c.lbattrs, "%EXTRA%", c.extra).Replace(complianceBundle)
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatal(err)
	}
	res, err := ingest.Ingest(dir, reg, 1)
	if err != nil || res.IR == nil {
		t.Fatalf("Ingest: %v %+v", err, res.Insufficient)
	}
	return res.IR
}

func statusOf(t *testing.T, results []core.ComplianceControlResult, control, node string) core.ComplianceStatus {
	t.Helper()
	for _, r := range results {
		if r.ControlID == control && r.NodeID == node {
			return r.Result.Status
		}
	}
	t.Fatalf("no result for %s on %q in %d results", control, node, len(results))
	return ""
}

const (
	// A private subnet's default route goes to a NAT gateway, never an internet gateway. (A route table
	// with no routes at all is not recognised as a route table, so "private" needs a real route.)
	privateRoute = `route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.n.id
  }`
	publicRoute = `route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }`
	closedSG = `ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    cidr_blocks     = ["10.0.0.0/16"]
  }`
	openIngressSG = `ingress {
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }`
	openEgressSG = closedSG + `
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }`
	httpsOnlyEgressSG = closedSG + `
  egress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }`
)

func TestPCI_1_3_1_and_1_3_2_SecurityGroupDirections(t *testing.T) {
	db := "aws_db_instance.d"
	cases := []struct {
		name, sg string
		in, out  core.ComplianceStatus
	}{
		{"closed ingress, no egress rules", closedSG, core.ComplianceApplicable, core.ComplianceApplicable},
		{"ingress open to the world", openIngressSG, core.ComplianceUnsatisfied, core.ComplianceApplicable},
		{"all-protocol egress to the world", openEgressSG, core.ComplianceApplicable, core.ComplianceUnsatisfied},
		{"https-only egress is not all-protocol", httpsOnlyEgressSG, core.ComplianceApplicable, core.ComplianceApplicable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := core.BuildPCIDSS4Catalog(complianceIR(t, cfg{route: privateRoute, sgrules: c.sg}), core.Workload{})
			if got := statusOf(t, res, "pci_dss_4:1.3.1", db); got != c.in {
				t.Errorf("1.3.1 = %s, want %s", got, c.in)
			}
			if got := statusOf(t, res, "pci_dss_4:1.3.2", db); got != c.out {
				t.Errorf("1.3.2 = %s, want %s", got, c.out)
			}
		})
	}
}

func TestPCI_1_4_4_and_SOC2_CC6_6_DatabaseReachability(t *testing.T) {
	db := "aws_db_instance.d"
	pub := complianceIR(t, cfg{route: publicRoute, sgrules: closedSG})
	priv := complianceIR(t, cfg{route: privateRoute, sgrules: closedSG})
	// 1.4.4 is assessable, so a private database is plainly satisfied; CC6.6 is partial, so it is applicable.
	if got := statusOf(t, core.BuildPCIDSS4Catalog(pub, core.Workload{}), "pci_dss_4:1.4.4", db); got != core.ComplianceUnsatisfied {
		t.Errorf("1.4.4 with a route to an internet gateway = %s, want unsatisfied", got)
	}
	if got := statusOf(t, core.BuildPCIDSS4Catalog(priv, core.Workload{}), "pci_dss_4:1.4.4", db); got != core.ComplianceSatisfied {
		t.Errorf("1.4.4 with no such route = %s, want satisfied", got)
	}
	if got := statusOf(t, core.BuildSOC2Catalog(pub, core.Workload{}), "soc2:CC6.6", db); got != core.ComplianceUnsatisfied {
		t.Errorf("CC6.6 public = %s, want unsatisfied", got)
	}
	if got := statusOf(t, core.BuildSOC2Catalog(priv, core.Workload{}), "soc2:CC6.6", db); got != core.ComplianceApplicable {
		t.Errorf("CC6.6 private = %s, want applicable (partial controls never read satisfied)", got)
	}
}

func TestPCI_3_5_1_2_and_CC6_1_StorageEncryptionIsSupportNeverAPassOrAFail(t *testing.T) {
	db := "aws_db_instance.d"
	enc := complianceIR(t, cfg{route: privateRoute, sgrules: closedSG, dbattrs: `storage_encrypted = true`})
	plain := complianceIR(t, cfg{route: privateRoute, sgrules: closedSG, dbattrs: `storage_encrypted = false`})
	unknown := complianceIR(t, cfg{route: privateRoute, sgrules: closedSG, dbattrs: `storage_encrypted = var.enc`})
	for _, id := range []string{"pci_dss_4:3.5.1.2", "soc2:CC6.1"} {
		build := func(ir *core.IR) []core.ComplianceControlResult {
			if strings.HasPrefix(id, "pci") {
				return core.BuildPCIDSS4Catalog(ir, core.Workload{})
			}
			return core.BuildSOC2Catalog(ir, core.Workload{})
		}
		if got := statusOf(t, build(enc), id, db); got != core.ComplianceApplicable {
			t.Errorf("%s encrypted = %s, want applicable (disk-level encryption supports but does not satisfy)", id, got)
		}
		if got := statusOf(t, build(plain), id, db); got != core.ComplianceNotAssessable {
			t.Errorf("%s unencrypted = %s, want not_assessable (field-level protection is invisible, so no verdict)", id, got)
		}
		if got := statusOf(t, build(unknown), id, db); got != core.ComplianceNotAssessable {
			t.Errorf("%s encryption unknown = %s, want not_assessable", id, got)
		}
	}
	// CIS judges the same unencrypted fact on its own terms.
	cis := core.BuildCISAWSCatalog(plain, core.Workload{})
	if got := statusOf(t, cis, "cis_aws.storage_encryption", db); got != core.ComplianceUnsatisfied {
		t.Errorf("CIS unencrypted = %s, want unsatisfied", got)
	}
}

func TestPCI_6_4_2_WebApplicationFirewall(t *testing.T) {
	lb := "aws_lb.front"
	waf := `
resource "aws_wafv2_web_acl" "w" {
  name  = "w"
  scope = "REGIONAL"
}
resource "aws_wafv2_web_acl_association" "assoc" {
  resource_arn = aws_lb.front.arn
  web_acl_arn  = aws_wafv2_web_acl.w.arn
}`
	cases := []struct {
		name  string
		c     cfg
		want  core.ComplianceStatus
		found bool
	}{
		{"internet-facing with a web ACL attached", cfg{lbattrs: `internal = false`, extra: waf}, core.ComplianceApplicable, true},
		{"default (internet-facing) with a web ACL attached", cfg{extra: waf}, core.ComplianceApplicable, true},
		{"internet-facing with none: absence is not a failure", cfg{lbattrs: `internal = false`}, core.ComplianceNotAssessable, true},
		{"internal load balancer is not public-facing, so no row", cfg{lbattrs: `internal = true`}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.c.route, c.c.sgrules = privateRoute, closedSG
			res := core.BuildPCIDSS4Catalog(complianceIR(t, c.c), core.Workload{})
			var found bool
			for _, r := range res {
				if r.ControlID == "pci_dss_4:6.4.2" && r.NodeID == lb {
					found = true
					if r.Result.Status != c.want {
						t.Errorf("6.4.2 = %s, want %s (%s)", r.Result.Status, c.want, r.Rationale)
					}
				}
			}
			if found != c.found {
				t.Errorf("a 6.4.2 row for the load balancer: found=%v, want %v", found, c.found)
			}
		})
	}
	// No public-facing load balancer at all: one not_assessable row, never a silent omission.
	res := core.BuildPCIDSS4Catalog(&core.IR{}, core.Workload{})
	if got := statusOf(t, res, "pci_dss_4:6.4.2", ""); got != core.ComplianceNotAssessable {
		t.Errorf("6.4.2 with nothing to evaluate = %s, want not_assessable", got)
	}
}

func TestSOC2_A1_2_RecoveryInfrastructure(t *testing.T) {
	db := "aws_db_instance.d"
	for name, c := range map[string]struct {
		attrs string
		want  core.ComplianceStatus
	}{
		"multi-AZ standby":                {`multi_az = true`, core.ComplianceApplicable},
		"single AZ: not a failure":        {`multi_az = false`, core.ComplianceNotAssessable},
		"multi-AZ declared by a variable": {`multi_az = var.ha`, core.ComplianceNotAssessable},
	} {
		t.Run(name, func(t *testing.T) {
			ir := complianceIR(t, cfg{route: privateRoute, sgrules: closedSG, dbattrs: c.attrs})
			if got := statusOf(t, core.BuildSOC2Catalog(ir, core.Workload{}), "soc2:A1.2", db); got != c.want {
				t.Errorf("A1.2 = %s, want %s", got, c.want)
			}
		})
	}
}

func TestPCI_7_2_2_and_CC6_3_LeastPrivilege(t *testing.T) {
	role := `
resource "aws_iam_role" "app" {
  name               = "app"
  assume_role_policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":\"ec2.amazonaws.com\"},\"Action\":\"sts:AssumeRole\"}]}"
}
resource "aws_iam_role_policy" "p" {
  name   = "p"
  role   = aws_iam_role.app.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Action = %ACTION%, Resource = %RESOURCE% }] })
}`
	mk := func(action, resource string) *core.IR {
		return complianceIR(t, cfg{route: privateRoute, sgrules: closedSG, extra: strings.NewReplacer("%ACTION%", action, "%RESOURCE%", resource).Replace(role)})
	}
	for name, c := range map[string]struct {
		ir   *core.IR
		want core.ComplianceStatus
	}{
		"wildcard administrator": {mk(`"*"`, `"*"`), core.ComplianceUnsatisfied},
		"scoped grant":           {mk(`["s3:GetObject"]`, `"arn:aws:s3:::b/*"`), core.ComplianceApplicable},
	} {
		t.Run(name, func(t *testing.T) {
			if got := statusOf(t, core.BuildPCIDSS4Catalog(c.ir, core.Workload{}), "pci_dss_4:7.2.2", "aws_iam_role.app"); got != c.want {
				t.Errorf("7.2.2 = %s, want %s", got, c.want)
			}
			if got := statusOf(t, core.BuildSOC2Catalog(c.ir, core.Workload{}), "soc2:CC6.3", "aws_iam_role.app"); got != c.want {
				t.Errorf("CC6.3 = %s, want %s", got, c.want)
			}
		})
	}
	// No identity node at all: a not_assessable row, never omitted.
	if got := statusOf(t, core.BuildPCIDSS4Catalog(&core.IR{}, core.Workload{}), "pci_dss_4:7.2.2", ""); got != core.ComplianceNotAssessable {
		t.Errorf("7.2.2 with no identity = %s, want not_assessable", got)
	}
}
