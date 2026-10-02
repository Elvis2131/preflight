// This file is PC-132: usage-based cost from declared traffic — NAT gateway data
// processing, internet egress, cross-AZ data transfer, and load balancer
// data-processed LCUs. Reuses ComputeJourneyFlow (PC-125) to know which hops
// actually carry traffic and EvaluateEgressRoute (PC-129) to know whether a hop
// egresses via a NAT gateway — zero new reachability logic, only a new cost
// classification layered on top of what those engines already compute.
//
// SKU MATCHING (verified against real AWS Bulk Price List data, 2026-10-01/02;
// manifest in cmd/runnerd/internal/pricingfetch/testdata/pc132_usage_type_rows.json):
//   - NAT data processing: AmazonEC2 offer, usagetype "<REGION>-NatGateway-Bytes",
//     operation "NatGateway", unit GB (eu-west-1: $0.048/GB). The offer ALSO holds
//     "<REGION>-RegionalNatGateway-Bytes" (operation "RegionalNatGateway"), a different
//     product — a substring match would hit it, so matching is exact on the
//     region-stripped usagetype plus operation.
//   - Cross-AZ: "<REGION>-DataTransfer-Regional-Bytes", unit GB (eu-west-1: $0.01/GB;
//     AWS's description says in/out/between AZs, i.e. the rate applies per direction —
//     this engine bills the declared volume once, a stated lower bound).
//   - Internet egress: lives in the AWSDataTransfer offer (NOT AmazonEC2),
//     "<REGION>-DataTransfer-Out-Bytes", transferType "AWS Outbound", TIERED by
//     beginRange/endRange (eu-west-1: $0.09 first 10 TB, $0.085, $0.07, $0.05). The
//     account-wide monthly free allowance is not modelled (stated in the reason).
//   - ALB: AWSELB "LCUUsage"/"LoadBalancing:Application", unit LCU-Hrs. Per AWS ELB
//     pricing, 1 LCU = 1 GB/hour processed bytes for EC2/container/IP targets
//     (0.4 GB/h for Lambda), and billing uses the HIGHEST of four dimensions — so
//     the bytes dimension alone is a lower bound, and Lambda targets are not modelled.
//
// A row whose unit is not the expected one is skipped, never multiplied blindly.
// Rows spanning more than one region with no way to choose are cost_unknown, never
// "first row wins".
//
// Monthly volume conversion (steady_rps -> monthly bytes) is tagged Kind=assumed —
// the Card's own explicit instruction, and (per core/report.go's own doc comment)
// the first real producer of Kind=assumed provenance anywhere in this codebase.
package core

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// SecondsPerMonthAssumption reuses HoursPerMonthAssumption (core/cost.go, THE one
// place a month-length assumption lives) rather than a second, independent
// month-length constant — same number, expressed in seconds for a per-second rate
// (steady_rps) instead of a per-hour one.
const SecondsPerMonthAssumption = HoursPerMonthAssumption * 3600

// UsageCostKind names which real AWS usage-based billing dimension one entry prices.
type UsageCostKind string

const (
	UsageCostNATDataProcessing UsageCostKind = "nat_data_processing"
	UsageCostInternetEgress    UsageCostKind = "internet_egress"
	UsageCostCrossAZTransfer   UsageCostKind = "cross_az_transfer"
	UsageCostLBDataProcessed   UsageCostKind = "lb_data_processed"
)

// UsageBasedCostEntry is one journey-hop's own usage-based charge (or honest
// not_assessable gap) — never a per-node figure, since these charges are inherently
// about traffic FLOWING somewhere, not a resource existing.
type UsageBasedCostEntry struct {
	JourneyID     string
	HopFrom       string
	HopTo         string
	Kind          UsageCostKind
	Decision      CostDecision
	MonthlyGB     float64 // 0 when Decision is CostUnknown
	MonthlyAmount float64 // 0 when Decision is CostUnknown
	Currency      string
	Reason        string // required when CostUnknown; a short descriptive note when priced
	SnapshotID    string
	Provenance    Provenance
}

// ComputeUsageBasedCost runs every declared journey's own hops (via ComputeJourneyFlow,
// so only hops that actually carry traffic under the current fault state are costed)
// and classifies each into zero or more usage-cost dimensions. A journey missing any
// required input (SteadyRPS, AvgRequestBytes, AvgResponseBytes) produces exactly one
// not_assessable entry naming the missing field — never a partial guess.
func ComputeUsageBasedCost(ir *IR, workload Workload, table PriceTable, killed map[string]bool) []UsageBasedCostEntry {
	// A workload that declares exactly one region is priced in it: a snapshot may carry several
	// regions (or just the one fetched), and "first row wins" is never acceptable. With several
	// regions declared there is no single region to choose, and rows spanning regions stay
	// cost_unknown (priceUsageEntry) rather than a guess.
	if len(workload.Regions) == 1 {
		table = regionTable(table, workload.Regions[0])
	}
	var out []UsageBasedCostEntry
	for _, j := range workload.Journeys {
		out = append(out, usageCostForJourney(ir, j, table, killed)...)
	}
	return out
}

func usageCostForJourney(ir *IR, j DeclaredJourney, table PriceTable, killed map[string]bool) []UsageBasedCostEntry {
	prov := NewProvenance(KindDerived, "core/usage_cost:"+j.ID)

	missing := missingUsageCostInput(j)
	if missing != "" {
		return []UsageBasedCostEntry{{
			JourneyID: j.ID, Decision: CostUnknown,
			Reason:     fmt.Sprintf("journey %q has no declared %s — usage-based cost requires it", j.ID, missing),
			SnapshotID: table.SnapshotID, Provenance: prov,
		}}
	}

	flow := ComputeJourneyFlow(ir, j, killed)
	monthlyGB, assumedProv := monthlyGigabytes(j, table.SnapshotID)
	egress := directionalEgressGB(j)

	var out []UsageBasedCostEntry
	for _, hop := range flow.Hops {
		if !hop.Allowed {
			break // traffic does not actually reach beyond a blocked hop
		}
		out = append(out, classifyHopUsageCost(ir, j.ID, hop, monthlyGB, egress, table, assumedProv)...)
	}
	return out
}

func missingUsageCostInput(j DeclaredJourney) string {
	switch {
	case j.SteadyRPS == nil:
		return "steady_rps"
	case j.AvgRequestBytes == nil:
		return "avg_request_bytes"
	case j.AvgResponseBytes == nil:
		return "avg_response_bytes"
	default:
		return ""
	}
}

// monthlyGigabytes converts steady_rps and the declared average request+response
// payload into a monthly data volume — Kind=assumed, per the Card's own explicit
// instruction ("monthly usage assumptions... are tagged assumed and shown").
func monthlyGigabytes(j DeclaredJourney, snapshotID string) (float64, Provenance) {
	prov := NewProvenance(KindAssumed, fmt.Sprintf("workload.yaml:journeys[%s].steady_rps", j.ID))
	prov.Reason = fmt.Sprintf(
		"monthly volume assumed as steady_rps (%.4g) x (avg_request_bytes + avg_response_bytes) x %.0f seconds/month (the same %.0f-hours/month convention HoursPerMonthAssumption already uses, in seconds)",
		*j.SteadyRPS, SecondsPerMonthAssumption, HoursPerMonthAssumption)
	bytesPerRequest := *j.AvgRequestBytes + *j.AvgResponseBytes
	totalBytes := *j.SteadyRPS * bytesPerRequest * SecondsPerMonthAssumption
	gb := totalBytes / (1024 * 1024 * 1024)
	return gb, prov
}

// egressVolumes splits the monthly volume by direction so only the bytes that leave AWS are
// billed as internet egress.
type egressVolumes struct{ RequestGB, ResponseGB float64 }

func directionalEgressGB(j DeclaredJourney) egressVolumes {
	perSecond := *j.SteadyRPS * SecondsPerMonthAssumption / (1024 * 1024 * 1024)
	return egressVolumes{RequestGB: perSecond * *j.AvgRequestBytes, ResponseGB: perSecond * *j.AvgResponseBytes}
}

func classifyHopUsageCost(ir *IR, journeyID string, hop JourneyHopFlow, monthlyGB float64, egress egressVolumes, table PriceTable, assumedProv Provenance) []UsageBasedCostEntry {
	var out []UsageBasedCostEntry

	// Internet egress: either side of the hop is the internet sentinel. AWS charges data
	// transfer OUT to the internet only; inbound transfer is free (EC2 pricing, "Data
	// Transfer"). On an internet-originated hop the bytes leaving AWS are the RESPONSE; on a
	// hop whose destination is the internet they are the REQUEST. Billing both directions
	// would overstate the cost by the free inbound bytes.
	if hop.From == JourneyInternetSentinel || hop.To == JourneyInternetSentinel {
		leaving := egress.ResponseGB
		if hop.To == JourneyInternetSentinel {
			leaving = egress.RequestGB
		}
		out = append(out, priceUsageEntry(journeyID, hop, UsageCostInternetEgress, leaving, table, assumedProv,
			matchInternetEgressRow))
	}

	// NAT data processing: the non-internet side's own subnet egresses via a NAT
	// gateway (PC-129's own EvaluateEgressRoute, reused verbatim).
	realSide := hop.From
	if realSide == JourneyInternetSentinel {
		realSide = hop.To
	}
	if hop.From != JourneyInternetSentinel && hop.To != JourneyInternetSentinel {
		// Resource-to-resource hop — NAT only applies if it actually leaves the VPC,
		// which a same-VPC hop never does. Skip; NAT is only ever costed on the
		// internet-egress leg itself (realSide's own egress route), not interior hops.
		realSide = ""
	}
	if realSide != "" {
		if subnetID, ok := resolveSubnetID(ir, realSide); ok {
			if allowed, targetKind, _ := EvaluateEgressRoute(ir, subnetID); allowed && targetKind == "nat_gateway" {
				out = append(out, priceUsageEntry(journeyID, hop, UsageCostNATDataProcessing, monthlyGB, table, assumedProv,
					matchNATDataProcessingRow))
			}
		}
	}

	// Cross-AZ transfer: both sides resolve to a real, different availability_zone.
	if hop.From != JourneyInternetSentinel && hop.To != JourneyInternetSentinel {
		fromAZ, fromOK := resolveAvailabilityZone(ir, hop.From)
		toAZ, toOK := resolveAvailabilityZone(ir, hop.To)
		switch {
		case !fromOK || !toOK:
			out = append(out, UsageBasedCostEntry{
				JourneyID: journeyID, HopFrom: hop.From, HopTo: hop.To, Kind: UsageCostCrossAZTransfer,
				Decision: CostUnknown, Reason: "one or both endpoints have no resolvable availability_zone",
				SnapshotID: table.SnapshotID, Provenance: NewProvenance(KindDerived, "core/usage_cost:cross-az:"+journeyID),
			})
		case fromAZ != toAZ:
			out = append(out, priceUsageEntry(journeyID, hop, UsageCostCrossAZTransfer, monthlyGB, table, assumedProv,
				matchCrossAZTransferRow))
		}
	}

	// LB data-processed LCU: either endpoint is a load_balancer node. Only the
	// data-processed dimension is modelled — new-connections/active-connections/
	// rule-evaluations LCU dimensions have no declared input this IR captures at all
	// (no connection-rate or rule-count concept exists), so they are never silently
	// estimated; this dimension alone is reported, never a combined "LCU cost."
	for _, side := range []string{hop.From, hop.To} {
		if side == JourneyInternetSentinel {
			continue
		}
		if n, ok := findNode(ir, side); ok && n.Type == NodeTypeLoadBalancer {
			out = append(out, priceUsageEntry(journeyID, hop, UsageCostLBDataProcessed, monthlyGB, table, assumedProv,
				matchLBDataProcessedRow))
		}
	}

	return out
}

func resolveAvailabilityZone(ir *IR, nodeID string) (string, bool) {
	subnetID, ok := resolveSubnetID(ir, nodeID)
	if !ok {
		return "", false
	}
	n, ok := findNode(ir, subnetID)
	if !ok {
		return "", false
	}
	az, ok := n.RawAttributes["availability_zone"].(string)
	if !ok || az == "" {
		return "", false
	}
	return az, true
}

func priceUsageEntry(journeyID string, hop JourneyHopFlow, kind UsageCostKind, monthlyGB float64, table PriceTable, assumedProv Provenance, match func(PriceRow) bool) UsageBasedCostEntry {
	prov := NewProvenance(KindDerived, fmt.Sprintf("core/usage_cost:%s:%s->%s", kind, hop.From, hop.To))
	unknown := func(reason string) UsageBasedCostEntry {
		return UsageBasedCostEntry{
			JourneyID: journeyID, HopFrom: hop.From, HopTo: hop.To, Kind: kind,
			Decision: CostUnknown, Reason: reason, SnapshotID: table.SnapshotID, Provenance: prov,
		}
	}

	var rows []PriceRow
	regions := map[string]bool{}
	for _, row := range table.Rows {
		if match(row) {
			rows = append(rows, row)
			regions[row.Region] = true
		}
	}
	if len(rows) == 0 {
		return unknown("no matching price row for this usage dimension in the pinned snapshot")
	}
	if len(regions) > 1 {
		return unknown("matching price rows span several regions and this usage entry has no region to choose between them")
	}

	if kind == UsageCostInternetEgress {
		amount, ok := tieredAmount(rows, monthlyGB)
		if !ok {
			return unknown("internet egress rows carry no usable tier bounds (begin_range/end_range) — cannot price the volume")
		}
		return UsageBasedCostEntry{
			JourneyID: journeyID, HopFrom: hop.From, HopTo: hop.To, Kind: kind,
			Decision: CostPriced, MonthlyGB: monthlyGB, MonthlyAmount: amount, Currency: rows[0].Currency,
			Reason:     fmt.Sprintf("%.4g GB/month (%s) priced across the published volume tiers; the account-wide monthly free allowance is not modelled", monthlyGB, assumedProv.Reason),
			SnapshotID: table.SnapshotID, Provenance: prov,
		}
	}

	// Several rows for one dimension (differing only by price) cannot be told apart
	// here: pick deterministically (lowest price first would hide a conflict), so
	// refuse instead when they disagree.
	price := rows[0].Price
	for _, r := range rows[1:] {
		if r.Price != price {
			return unknown("several matching price rows disagree on the rate — refusing to pick one")
		}
	}
	reason := fmt.Sprintf("%.4g GB/month (%s) at $%.4f/GB", monthlyGB, assumedProv.Reason, price)
	if kind == UsageCostLBDataProcessed {
		reason = fmt.Sprintf("%.4g LCU-hours/month from the processed-bytes dimension only (1 LCU = 1 GB/hour for EC2/IP targets; billing uses the highest of four dimensions, so this is a lower bound) (%s) at $%.4f/LCU-hour", monthlyGB, assumedProv.Reason, price)
	}
	return UsageBasedCostEntry{
		JourneyID: journeyID, HopFrom: hop.From, HopTo: hop.To, Kind: kind,
		Decision: CostPriced, MonthlyGB: monthlyGB, MonthlyAmount: monthlyGB * price, Currency: rows[0].Currency,
		Reason: reason, SnapshotID: table.SnapshotID, Provenance: prov,
	}
}

// tieredAmount walks volume tiers in ascending begin_range order. ok is false when
// any row lacks parseable bounds.
func tieredAmount(rows []PriceRow, gb float64) (float64, bool) {
	type tier struct{ begin, end, price float64 }
	var tiers []tier
	for _, r := range rows {
		begin, err := strconv.ParseFloat(r.SKUAttributes["begin_range"], 64)
		if err != nil {
			return 0, false
		}
		end := math.Inf(1)
		if e := r.SKUAttributes["end_range"]; e != "" && e != "Inf" {
			if end, err = strconv.ParseFloat(e, 64); err != nil {
				return 0, false
			}
		}
		tiers = append(tiers, tier{begin, end, r.Price})
	}
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].begin < tiers[j].begin })
	var total float64
	for _, t := range tiers {
		if gb <= t.begin {
			break
		}
		total += (math.Min(gb, t.end) - t.begin) * t.price
	}
	return total, true
}

// usageTypeIs matches a usagetype exactly after dropping AWS's REGION prefix ("EU-", "USE1-", or
// none for us-east-1) — so "RegionalNatGateway-Bytes" does not match "NatGateway-Bytes". Only a
// genuine region prefix is dropped: short and all upper-case/digits. That matters: the real
// eu-west-1 ELB file also holds "EU-Outposts-LCUUsage" (a $0 Outposts rate) and
// "EU-ReservedLCUUsage", and neither is the on-demand "EU-LCUUsage".
func usageTypeIs(r PriceRow, want string) bool {
	u := r.SKUAttributes["usagetype"]
	if i := strings.Index(u, "-"); i > 0 && isRegionPrefix(u[:i]) {
		u = u[i+1:]
	}
	return u == want
}

func isRegionPrefix(s string) bool {
	if len(s) == 0 || len(s) > 6 {
		return false
	}
	for _, c := range s {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// matchNATDataProcessingRow/matchInternetEgressRow/matchCrossAZTransferRow/
// matchLBDataProcessedRow — see this file's own header SKU-matching note.
func matchNATDataProcessingRow(r PriceRow) bool {
	return usageTypeIs(r, "NatGateway-Bytes") && r.SKUAttributes["operation"] == "NatGateway" && r.Unit == "GB"
}

func matchInternetEgressRow(r PriceRow) bool {
	return usageTypeIs(r, "DataTransfer-Out-Bytes") && r.SKUAttributes["transferType"] == "AWS Outbound" && r.Unit == "GB"
}

func matchCrossAZTransferRow(r PriceRow) bool {
	return usageTypeIs(r, "DataTransfer-Regional-Bytes") && r.Unit == "GB"
}

func matchLBDataProcessedRow(r PriceRow) bool {
	return usageTypeIs(r, "LCUUsage") && r.SKUAttributes["operation"] == "LoadBalancing:Application" && r.Unit == "LCU-Hrs"
}
