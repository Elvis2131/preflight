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
	Service       string
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
func ComputeCost(ir *IR, table PriceTable, prov Provenance) CostReport {
	report := CostReport{SnapshotID: table.SnapshotID, HoursPerMonthAssumed: HoursPerMonthAssumption, Currency: "USD"}

	for _, n := range ir.Nodes {
		cc := costOneNode(n, table, prov)
		report.Components = append(report.Components, cc)
		if cc.Decision == CostPriced {
			report.PricedTotal += cc.MonthlyAmount
		} else {
			report.UnpricedCount++
		}
	}
	return report
}

func costOneNode(n Node, table PriceTable, prov Provenance) ComponentCost {
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
	case NodeTypeManagedDatabase:
		row, ok, reason = matchRDSRow(n, table)
	case NodeTypeCache:
		row, ok, reason = matchElastiCacheRow(n, table)
	case NodeTypeLoadBalancer:
		row, ok, reason = matchALBRow(n, table)
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
	base.SKURateCode = row.SKUAttributes["rate_description"]
	return base
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
