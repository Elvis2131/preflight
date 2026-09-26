// This file is PC-132: usage-based cost from declared traffic — NAT gateway data
// processing, internet egress, cross-AZ data transfer, and load balancer
// data-processed LCUs. Reuses ComputeJourneyFlow (PC-125) to know which hops
// actually carry traffic and EvaluateEgressRoute (PC-129) to know whether a hop
// egresses via a NAT gateway — zero new reachability logic, only a new cost
// classification layered on top of what those engines already compute.
//
// SKU MATCHING HONESTY NOTE: the usagetype strings this file matches against
// (NatGateway-Bytes, DataTransfer-Regional-Bytes, DataTransfer-Out-Bytes, LCUUsage)
// are AWS's own well-documented, standard Cost & Usage Report / Cost Explorer usage
// type vocabulary (publicly referenced across AWS's own billing documentation and
// tooling) — but unlike core/cost.go's own existing LoadBalancerUsage/RDS/ElastiCache
// matches, these were not cross-checked against one specific captured real Price
// List API response before writing this file (no live snapshot pull was available).
// Flagged explicitly, the same "uncertain stays unknown, don't silently guess and
// call it verified" discipline PC-18 already applied to a CIS control ID: verify
// these against a real pulled snapshot (pricing/testdata or a live pricingfetch run)
// before trusting this against production billing data.
//
// Monthly volume conversion (steady_rps -> monthly bytes) is tagged Kind=assumed —
// the Card's own explicit instruction, and (per core/report.go's own doc comment)
// the first real producer of Kind=assumed provenance anywhere in this codebase.
package core

import (
	"fmt"
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

	var out []UsageBasedCostEntry
	for _, hop := range flow.Hops {
		if !hop.Allowed {
			break // traffic does not actually reach beyond a blocked hop
		}
		out = append(out, classifyHopUsageCost(ir, j.ID, hop, monthlyGB, table, assumedProv)...)
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

func classifyHopUsageCost(ir *IR, journeyID string, hop JourneyHopFlow, monthlyGB float64, table PriceTable, assumedProv Provenance) []UsageBasedCostEntry {
	var out []UsageBasedCostEntry

	// Internet egress: either side of the hop is the internet sentinel.
	if hop.From == JourneyInternetSentinel || hop.To == JourneyInternetSentinel {
		out = append(out, priceUsageEntry(journeyID, hop, UsageCostInternetEgress, monthlyGB, table, assumedProv,
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
	for _, row := range table.Rows {
		if match(row) {
			return UsageBasedCostEntry{
				JourneyID: journeyID, HopFrom: hop.From, HopTo: hop.To, Kind: kind,
				Decision: CostPriced, MonthlyGB: monthlyGB, MonthlyAmount: monthlyGB * row.Price, Currency: row.Currency,
				Reason:     fmt.Sprintf("%.4g GB/month (%s) at $%.4f/GB", monthlyGB, assumedProv.Reason, row.Price),
				SnapshotID: table.SnapshotID, Provenance: prov,
			}
		}
	}
	return UsageBasedCostEntry{
		JourneyID: journeyID, HopFrom: hop.From, HopTo: hop.To, Kind: kind,
		Decision: CostUnknown, Reason: "no matching price row for this usage dimension in the pinned snapshot",
		SnapshotID: table.SnapshotID, Provenance: prov,
	}
}

// matchNATDataProcessingRow/matchInternetEgressRow/matchCrossAZTransferRow/
// matchLBDataProcessedRow — see this file's own header SKU-matching honesty note.
func matchNATDataProcessingRow(r PriceRow) bool {
	return containsFold(r.SKUAttributes["usagetype"], "NatGateway-Bytes")
}

func matchInternetEgressRow(r PriceRow) bool {
	return containsFold(r.SKUAttributes["usagetype"], "DataTransfer-Out-Bytes")
}

func matchCrossAZTransferRow(r PriceRow) bool {
	return containsFold(r.SKUAttributes["usagetype"], "DataTransfer-Regional-Bytes")
}

func matchLBDataProcessedRow(r PriceRow) bool {
	return containsFold(r.SKUAttributes["usagetype"], "LCUUsage") && r.SKUAttributes["operation"] == "LoadBalancing:Application"
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
