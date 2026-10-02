// This file is PC-117: cost computation, pure in core (I1) — inputs are the IR (with
// PC-115's Sizing) and a pricing snapshot's price table, both passed in by the caller
// (server); core never fetches prices, never imports pricing/ or cmd/runnerd/internal/
// pricingfetch, and makes no network call of its own. PriceTable/PriceRow below are
// core's own plain mirror of pricing.Snapshot/PriceEntry — core cannot import pricing/
// (I1: core is the pure engine everything else depends ON, never the reverse, the same
// direction core/routing.go's UnsupportedRouteInfo already established for ingest) —
// the caller (server.Assess) converts a real pricing.Snapshot into this shape.
package core

import "strings"

// HoursPerMonthAssumption is THE one place this constant lives (the Card's own
// instruction) — AWS's own commonly-cited average (365.25 days/year ÷ 12 months × 24
// hours), used to convert an hourly rate into a monthly estimate. This is a stated
// assumption, not a measured fact, and every CostReport it feeds into says so.
const HoursPerMonthAssumption = 730.0

// PriceRow is core's own plain mirror of pricing.PriceEntry.
type PriceRow struct {
	Service string
	// Region is the AWS region code the row prices (PC-110). Empty means the snapshot
	// recorded no region for it (see priceRowInRegion).
	Region        string
	SKUAttributes map[string]string
	Unit          string
	Price         float64
	Currency      string
}

// PriceTable is core's own plain mirror of a pricing.Snapshot — SnapshotID plus its
// full row set, exactly what the caller pins an assessment to.
type PriceTable struct {
	SnapshotID string
	Rows       []PriceRow
}

// CostDecision is I4 applied to cost: a component's price is either known or it isn't
// — never a guessed default standing in for "we don't actually know."
type CostDecision string

const (
	CostPriced  CostDecision = "priced"
	CostUnknown CostDecision = "cost_unknown"
)

// ComponentCost is one node's own costed (or honestly not_assessable) result —
// PC-117's own acceptance criterion: "Every cost figure is a provenance-tagged value
// citing the snapshot ID and the SKU/price row it used."
type ComponentCost struct {
	NodeID        string
	Decision      CostDecision
	MonthlyAmount float64 // 0 when Decision is CostUnknown — never a guessed price
	Currency      string
	Reason        string // required when CostUnknown: WHY (missing sizing, service out of scope, ...); a short descriptive note when priced
	SnapshotID    string
	SKURateCode   string // the specific AWS rateCode this figure came from — empty when CostUnknown
	Provenance    Provenance

	// Region/RegionSource are PC-110's own addition: the region this component was priced
	// in and WHICH of the three resolution steps supplied it (component | workload).
	// Empty when no region was resolved (the component is then cost_unknown, Reason naming
	// "region") or when the node type is not priced at all.
	Region       string       `json:",omitempty"`
	RegionSource RegionSource `json:",omitempty"`
}

// CostReport is the whole assessment's cost picture — PC-117's own acceptance
// criterion: "Total is reported as 'priced total' plus an explicit count/list of
// unpriced components. Never present a total as complete when components were
// excluded." PricedTotal sums ONLY Components with Decision==CostPriced; a caller
// that ignores UnpricedCount and reports PricedTotal alone as "the cost" is
// misusing this type, not a shape this type itself permits silently — Components
// always carries every node, priced and unpriced together, specifically so that
// omission is visible, not just aggregatable-away.
type CostReport struct {
	SnapshotID           string
	HoursPerMonthAssumed float64
	PricedTotal          float64
	Currency             string
	Components           []ComponentCost
	UnpricedCount        int

	// BudgetUSD/BudgetPriority/BudgetExceeded are PC-118's own addition — populated by
	// ApplyCostBudget when the workload declares a "monthly_cost_budget_usd"
	// Requirement (BudgetUSD stays nil when no such requirement was declared, never a
	// guessed/default budget). A PREFERENCE-priority budget being exceeded is
	// recorded here as a trade-off fact, deliberately never surfaced as a compliance
	// Finding (Card: "not automatically a regression") — only a HARD budget's
	// violation becomes a real Finding, see BuildCostBudgetFindings.
	BudgetUSD      *float64            `json:"budget_usd,omitempty"`
	BudgetPriority RequirementPriority `json:"budget_priority,omitempty"`
	BudgetExceeded bool                `json:"budget_exceeded,omitempty"`
}

// engineNameToAWS normalizes a Terraform aws_db_instance `engine` value to the AWS
// Price List's own `databaseEngine` attribute value — verified against a real,
// live-fetched AmazonRDS price row (instanceType=db.r6g.xlarge, engine="postgres" in
// Terraform -> databaseEngine="PostgreSQL" in the real price data, 2026-09-25).
// Deliberately small: only engines this codebase has verified a real match for.
// Widening it is real, additional verification work, not free — an unrecognized
// engine stays honestly cost_unknown rather than guessed.
var engineNameToAWS = map[string]string{
	"postgres": "PostgreSQL",
	"mysql":    "MySQL",
	"mariadb":  "MariaDB",
}

// ComputeCost is PC-117's pure computation: same IR + same PriceTable, always
// byte-identical output (no clock read, no randomness, no map-iteration-order
// dependence — Components is built in a single pass over ir.Nodes, itself already in
// a stable, sorted order per ingest.Ingest's own sort.Slice by ID).
func ComputeCost(ir *IR, table PriceTable, workloadRegions []string, prov Provenance) CostReport {
	report := CostReport{SnapshotID: table.SnapshotID, HoursPerMonthAssumed: HoursPerMonthAssumption, Currency: "USD"}

	nats := natGatewayIDs(ir)
	for _, n := range ir.Nodes {
		var cc ComponentCost
		if nats[n.ID] {
			cc = costNATGateway(n, table, workloadRegions, prov)
		} else {
			cc = costOneNode(n, table, workloadRegions, prov)
		}
		report.Components = append(report.Components, cc)
		if cc.Decision == CostPriced {
			report.PricedTotal += cc.MonthlyAmount
		} else {
			report.UnpricedCount++
		}
	}
	return report
}

func costOneNode(n Node, table PriceTable, workloadRegions []string, prov Provenance) ComponentCost {
	base := ComponentCost{NodeID: n.ID, SnapshotID: table.SnapshotID, Currency: "USD", Provenance: prov}

	if n.Sizing == nil {
		base.Decision = CostUnknown
		base.Reason = "no sizing declared for this component (PC-115) — cost_unknown, never a default"
		return base
	}

	var row PriceRow
	var ok bool
	var reason string
	switch n.Type {
	case NodeTypeManagedDatabase, NodeTypeCache, NodeTypeLoadBalancer:
		// PC-110: price only in a region that actually resolved — never a guessed one.
		region, source, why := ResolveComponentRegion(n.Sizing, workloadRegions)
		if region == "" {
			base.Decision = CostUnknown
			base.Reason = why
			return base
		}
		base.Region, base.RegionSource = region, source
		switch n.Type {
		case NodeTypeManagedDatabase:
			row, ok, reason = matchRDSRow(n, regionTable(table, region))
		case NodeTypeCache:
			row, ok, reason = matchElastiCacheRow(n, regionTable(table, region))
		default:
			row, ok, reason = matchALBRow(n, regionTable(table, region))
		}
		if !ok && reason != "" {
			reason += " (region " + region + ")"
		}
	default:
		reason = "service not in this pricing snapshot's scope (ADR-006 §3)"
	}

	if !ok {
		base.Decision = CostUnknown
		if reason == "" {
			reason = "no matching price row in this snapshot"
		}
		base.Reason = reason
		return base
	}

	base.Decision = CostPriced
	base.MonthlyAmount = row.Price * HoursPerMonthAssumption
	// ElastiCache's price row is a PER-NODE hourly rate — a replication group with
	// multiple nodes (Sizing.Count, from num_cache_clusters) costs that many times
	// over, a real fact the golden bundle's own aws-broken (1 node) vs aws (3 nodes)
	// difference makes directly observable, not a hypothetical.
	if n.Type == NodeTypeCache && n.Sizing.Count != nil && *n.Sizing.Count > 0 {
		base.MonthlyAmount *= float64(*n.Sizing.Count)
	}
	base.SKURateCode = row.SKUAttributes["rate_description"]
	return base
}

// regionTable returns table restricted to rows that may price region (priceRowInRegion).
func regionTable(table PriceTable, region string) PriceTable {
	out := PriceTable{SnapshotID: table.SnapshotID}
	for _, r := range table.Rows {
		if priceRowInRegion(r, region) {
			out.Rows = append(out.Rows, r)
		}
	}
	return out
}

func matchRDSRow(n Node, table PriceTable) (PriceRow, bool, string) {
	if n.Sizing.InstanceClass == nil {
		return PriceRow{}, false, "no instance_class declared — cost_unknown"
	}
	engineRaw, _ := n.RawAttributes["engine"].(string)
	engine, known := engineNameToAWS[engineRaw]
	if !known {
		return PriceRow{}, false, "database engine \"" + engineRaw + "\" is not in this fetcher's verified engine-name mapping — cost_unknown, not guessed"
	}
	multiAZ, _ := n.RawAttributes["multi_az"].(bool)
	deployment := "Single-AZ"
	if multiAZ {
		deployment = "Multi-AZ"
	}

	for _, row := range table.Rows {
		if row.Service != "AmazonRDS" || row.Unit != "Hrs" {
			continue
		}
		a := row.SKUAttributes
		if a["instanceType"] == *n.Sizing.InstanceClass && a["databaseEngine"] == engine && a["deploymentOption"] == deployment {
			return row, true, ""
		}
	}
	return PriceRow{}, false, "instance class \"" + *n.Sizing.InstanceClass + "\" (engine " + engine + ", " + deployment + ") not found in this pricing snapshot"
}

func matchElastiCacheRow(n Node, table PriceTable) (PriceRow, bool, string) {
	if n.Sizing.CacheNodeType == nil {
		return PriceRow{}, false, "no cache_node_type declared — cost_unknown"
	}
	// Real AWS ElastiCache engine attribute is capitalized ("Redis", "Valkey",
	// "Memcached") — this codebase currently only ingests "redis"/"memcached"-style
	// lowercase Terraform values; only Redis is verified end-to-end so far (golden
	// bundle), same "small, verified, honestly incomplete" discipline as RDS's own
	// engine map.
	engineRaw, _ := n.RawAttributes["engine"].(string)
	var engine string
	switch strings.ToLower(engineRaw) {
	case "redis":
		engine = "Redis"
	case "memcached":
		engine = "Memcached"
	default:
		return PriceRow{}, false, "cache engine \"" + engineRaw + "\" is not in this fetcher's verified engine-name mapping — cost_unknown, not guessed"
	}

	for _, row := range table.Rows {
		if row.Service != "AmazonElastiCache" || row.Unit != "Hrs" {
			continue
		}
		a := row.SKUAttributes
		// The real snapshot carries multiple usagetype variants per instance
		// type/engine (extended-support-year pricing, etc.) — the plain
		// "NodeUsage:<type>" usagetype (no region/year prefix) is the real, standard
		// on-demand rate, verified against a live-fetched row, 2026-09-25.
		if a["instanceType"] == *n.Sizing.CacheNodeType && a["cacheEngine"] == engine && a["usagetype"] == "NodeUsage:"+*n.Sizing.CacheNodeType {
			return row, true, ""
		}
	}
	return PriceRow{}, false, "cache node type \"" + *n.Sizing.CacheNodeType + "\" (engine " + engine + ") not found in this pricing snapshot"
}

func matchALBRow(n Node, table PriceTable) (PriceRow, bool, string) {
	if n.Sizing.LoadBalancerType == nil {
		return PriceRow{}, false, "no load_balancer_type declared — cost_unknown"
	}
	if *n.Sizing.LoadBalancerType != "application" {
		return PriceRow{}, false, "load_balancer_type \"" + *n.Sizing.LoadBalancerType + "\" pricing is not yet verified (only \"application\"/ALB is) — cost_unknown"
	}
	for _, row := range table.Rows {
		if row.Service != "AWSELB" || row.Unit != "Hrs" {
			continue
		}
		a := row.SKUAttributes
		// The real hourly base rate is the plain, regional "LoadBalancerUsage" row
		// (as opposed to LCU usage-based charges, Outposts, or TS/trial-service
		// variants) — verified against a live-fetched row, 2026-09-25. Usage-based
		// LCU/data-processing charges stay not_assessable until PC-124's traffic
		// inputs exist, per the Card's own instruction — never estimated here.
		if a["usagetype"] == "LoadBalancerUsage" && a["operation"] == "LoadBalancing:Application" {
			return row, true, ""
		}
	}
	return PriceRow{}, false, "no base Application Load Balancer hourly rate found in this pricing snapshot"
}

// CostBudgetRequirementID is the one recognized Workload.Requirements[].ID for a
// declared monthly cost budget — PC-118's own Card: "Cost is typically a preference-
// priority requirement in the workload." Reuses PRD §4's existing, general
// Requirement{ID, Value, Priority} mechanism rather than inventing a dedicated
// Workload field — Value must be a float64 (a monthly USD amount); Priority follows
// PRD §4's own hard/preference vocabulary unchanged.
const CostBudgetRequirementID = "monthly_cost_budget_usd"

func findCostBudgetRequirement(requirements []Requirement) (amount float64, priority RequirementPriority, ok bool) {
	for _, r := range requirements {
		if r.ID != CostBudgetRequirementID {
			continue
		}
		// A YAML-decoded requirements value carries whichever concrete numeric Go
		// type its literal in the source file happened to be (a bare integer like
		// "500" decodes to int, "500.0" to float64) — both are real, honest ways for
		// an architect to have declared the same budget, so both are accepted here
		// rather than silently treating an integer-literal budget as undeclared.
		switch v := r.Value.(type) {
		case float64:
			return v, r.Priority, true
		case int:
			return float64(v), r.Priority, true
		}
	}
	return 0, "", false
}

// ApplyCostBudget evaluates a declared cost budget requirement (if any) against an
// already-computed CostReport, returning an augmented copy — a pure function, called
// after ComputeCost, never inside it (ComputeCost's own inputs stay IR+PriceTable
// only, per its own doc comment). A report with no declared budget requirement is
// returned completely unchanged (BudgetUSD stays nil).
func ApplyCostBudget(report CostReport, requirements []Requirement) CostReport {
	amount, priority, ok := findCostBudgetRequirement(requirements)
	if !ok {
		return report
	}
	report.BudgetUSD = &amount
	report.BudgetPriority = priority
	report.BudgetExceeded = report.PricedTotal > amount
	return report
}

// natGatewayIDs is the set of nodes that ARE NAT gateways, found structurally: the target of a
// route whose target_kind is "nat_gateway" (ingest/routes.go) — never by a provider resource
// name (I1).
func natGatewayIDs(ir *IR) map[string]bool {
	out := map[string]bool{}
	for _, e := range ir.Edges {
		if e.Type != EdgeTypeRoutesTo {
			continue
		}
		if kind, _ := e.RawAttributes["target_kind"].(string); kind == "nat_gateway" {
			out[e.To] = true
		}
	}
	return out
}

// costNATGateway prices a NAT gateway's HOURLY charge (AmazonEC2 offer, productFamily "NAT Gateway",
// usagetype "<REGION>-NatGateway-Hours", unit Hrs — ADR-006 amendment, 2026-10-02). It needs no
// sizing: the price depends only on the region. Data processing is a usage charge and is priced
// from declared traffic elsewhere (usage_cost.go). With no resolvable region, or no matching row,
// it is cost_unknown with a stated reason — never a default, never zero.
func costNATGateway(n Node, table PriceTable, workloadRegions []string, prov Provenance) ComponentCost {
	base := ComponentCost{NodeID: n.ID, SnapshotID: table.SnapshotID, Currency: "USD", Provenance: prov}
	region, source, why := ResolveComponentRegion(n.Sizing, workloadRegions)
	if region == "" {
		base.Decision, base.Reason = CostUnknown, why
		return base
	}
	base.Region, base.RegionSource = region, source
	var prices []float64
	var rate string
	for _, r := range regionTable(table, region).Rows {
		if usageTypeIs(r, "NatGateway-Hours") && r.SKUAttributes["operation"] == "NatGateway" && r.Unit == "Hrs" {
			prices = append(prices, r.Price)
			rate = r.SKUAttributes["rate_description"]
		}
	}
	switch {
	case len(prices) == 0:
		base.Decision, base.Reason = CostUnknown, "no NAT gateway hourly price row for region "+region+" in this snapshot (ADR-006 amendment: the snapshot stores AmazonEC2's NAT Gateway family only when it was fetched)"
	case !allEqual(prices):
		base.Decision, base.Reason = CostUnknown, "several NAT gateway hourly rows disagree on the rate for region "+region+" — refusing to pick one"
	default:
		base.Decision = CostPriced
		base.MonthlyAmount = prices[0] * HoursPerMonthAssumption
		base.SKURateCode = rate
	}
	return base
}

func allEqual(v []float64) bool {
	for _, x := range v[1:] {
		if x != v[0] {
			return false
		}
	}
	return true
}
