// This file is PC-122's own core-side half: a pure, deterministic HTML rendering of
// core.Report. Renders layout only, per the Card's own explicit instruction — every
// value on the page already exists on the Report struct; this file computes nothing
// new (no verdict, no derived number, no content decision beyond "which template
// slot does this field go in"). Pure Go html/template — no subprocess, no I/O — so
// this can live in core (I1) exactly like core/graph.go's own RenderDOT; only the
// PDF conversion step (an external tool) needs the subprocess boundary render/
// already established for SVG (PC-81), and lives there instead, not here.
//
// Determinism: html/template's own `range` over a Go map iterates in SORTED key
// order (a documented Go stdlib guarantee, text/template's own spec, since Go 1.12) —
// so ExecutiveSummary.ScorecardStatusCounts/ComplianceCatalogCounts/
// ComplianceResultCounts (all maps) render in stable order with no explicit sort
// needed here. Every other field this template walks is already a slice (stable,
// caller-determined order) or a scalar. No clock read, no randomness — the exact
// same guarantee core.RenderDOT's own doc comment already states for the diagram.
package core

import (
	"bytes"
	"html/template"
)

// reportHTMLTemplate is deliberately one literal constant, not loaded from disk —
// same "no external file the binary could silently drift from" reasoning
// core/graph.go's own DOT template already follows. Mandatory content, per the
// Card's own explicit list, each marked below with which section renders it: cost
// disclaimer, not_assessable reasons, compliance not-assessable-from-architecture
// counts, and the assumptions appendix.
const reportHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Architecture Assurance Report — {{.SessionID}} v{{.VersionNumber}}</title>
<style>
body{font-family:Georgia,serif;max-width:960px;margin:2rem auto;padding:0 1rem;color:#0f172a}
h1,h2,h3{font-family:Arial,sans-serif}
h1{font-size:1.6rem;border-bottom:2px solid #0f172a;padding-bottom:.5rem}
h2{font-size:1.2rem;margin-top:2rem;border-bottom:1px solid #cbd5e1;padding-bottom:.25rem}
table{border-collapse:collapse;width:100%;margin:.5rem 0}
th,td{border:1px solid #cbd5e1;padding:.35rem .5rem;font-size:.85rem;text-align:left;vertical-align:top}
th{background:#f1f5f9}
.disclaimer{background:#fef3c7;border:1px solid #d97706;padding:.75rem;font-size:.85rem;margin:.5rem 0}
.not-assessable{color:#92400e}
.reason{font-style:italic;color:#64748b}
.diagram{border:1px solid #cbd5e1;padding:.5rem;overflow:auto}
code{background:#f1f5f9;padding:0 .2rem}
</style>
</head>
<body>
<h1>Architecture Assurance Report</h1>
<p>Session <code>{{.SessionID}}</code> — version {{.VersionNumber}}</p>

<h2>Diagram</h2>
<div class="diagram">{{.Graph}}</div>

<h2>Executive summary</h2>
<p>Per-dimension counts only — no composite architecture score.</p>
<h3>Scorecard status counts</h3>
<table><tr><th>Status</th><th>Count</th></tr>
{{range $status, $count := .ExecutiveSummary.ScorecardStatusCounts}}<tr><td>{{$status}}</td><td>{{$count}}</td></tr>
{{end}}</table>
<h3>NFR conformance</h3>
<p>{{.ExecutiveSummary.NFREvaluatedCount}} evaluated, {{.ExecutiveSummary.NFRNotEvaluatedCount}} not evaluated.</p>
<h3>Compliance catalog counts</h3>
<table><tr><th>Framework</th><th>Assessable</th><th>Partial</th><th class="not-assessable">Not assessable from architecture</th></tr>
{{range $fw, $c := .ExecutiveSummary.ComplianceCatalogCounts}}<tr><td>{{$fw}}</td><td>{{$c.Assessable}}</td><td>{{$c.Partial}}</td><td class="not-assessable">{{$c.NotAssessable}}</td></tr>
{{end}}</table>

<h2>NFR conformance</h2>
<table><tr><th>Requirement</th><th>Priority</th><th>Value</th><th>Evaluated</th><th>Finding(s) / reason</th></tr>
{{range .NFRConformance}}<tr>
<td>{{.RequirementID}}</td><td>{{.Priority}}</td><td>{{.Value}}</td><td>{{.Evaluated}}</td>
<td>{{if .Evaluated}}{{range .FindingIDs}}<code>{{.}}</code> {{end}}{{else}}<span class="reason">{{.Reason}}</span>{{end}}</td>
</tr>{{end}}</table>

<h2>Compliance by framework</h2>
{{range .Compliance}}
<h3>{{.Framework}}</h3>
<table><tr><th>Control</th><th>Requirement</th><th>Title</th><th>Classification</th><th>Status</th><th>Rationale</th></tr>
{{range .Controls}}<tr>
<td><code>{{.ControlID}}</code></td><td>{{.RequirementID}}</td><td>{{.Title}}</td><td>{{.Classification}}</td>
<td{{if eq (print .Result.Status) "not_assessable"}} class="not-assessable"{{end}}>{{.Result.Status}}</td>
<td class="reason">{{.Rationale}}</td>
</tr>{{end}}</table>
{{end}}

<h2>Failure modes</h2>
{{if .FailureModes.Available}}
<table><tr><th>Finding</th><th>Trigger</th><th>Detection</th><th>Impact</th><th>Outcome</th></tr>
{{range .FailureModes.Findings}}<tr>
<td><code>{{.ID}}</code><br>{{.Title}}</td>
<td>{{.Dimensions.Trigger}}</td>
<td>{{.Dimensions.Detection}}</td>
<td{{if eq (print .Dimensions.Impact.State) "not_assessable"}} class="not-assessable"{{end}}>
{{if eq (print .Dimensions.Impact.State) "not_assessable"}}<span class="reason">not_assessable: {{.Dimensions.Impact.Reason}}</span>{{else}}{{.Dimensions.Impact.Value}}{{end}}
</td>
<td{{if eq (print .Outcome.State) "not_assessable"}} class="not-assessable"{{end}}>
{{if eq (print .Outcome.State) "not_assessable"}}<span class="reason">not_assessable: {{.Outcome.Reason}}</span>{{else}}{{.Outcome.Value}}{{end}}
</td>
</tr>{{end}}</table>
{{else}}<p class="reason not-assessable">Not available: {{.FailureModes.UnavailableReason}}</p>{{end}}

{{if .Scenarios}}<h2>Saved failure scenarios</h2>
<p class="reason">Each scenario is re-run against this version; results are never carried over from an earlier one.</p>
<table><tr><th>scenario</th><th>verdict</th><th>failed journeys</th><th>degraded journeys</th><th>severed paths</th></tr>
{{range .Scenarios}}<tr><td>{{.Name}}</td><td>{{if eq .Verdict.State "assessed"}}{{.Verdict.Value}}{{else}}not assessable: {{.Verdict.Reason}}{{end}}</td><td>{{range .FailedJourneys}}{{.}} {{end}}</td><td>{{range .DegradedJourneys}}{{.}} {{end}}</td><td>{{range .SeveredPaths}}{{.}} {{end}}</td></tr>{{end}}</table>
{{end}}<h2>Traffic and capacity</h2>
{{if .Traffic.Available}}
<table><tr><th>Journey</th><th>Flows</th><th>Blocked at</th><th>Reason</th></tr>
{{range .Traffic.Flows}}<tr><td>{{.JourneyID}}</td><td>{{.Flows}}</td><td>{{.BlockedAt}}</td><td class="reason">{{.BlockedReason}}</td></tr>{{end}}
</table>
{{else}}<p class="reason not-assessable">Not available: {{.Traffic.UnavailableReason}}</p>{{end}}

<h2>Cost</h2>
<div class="disclaimer">{{.Cost.Disclaimer}}</div>
{{if .Cost.Available}}
<p>Priced total: {{.Cost.Report.Currency}} {{.Cost.Report.PricedTotal}} (snapshot <code>{{.Cost.Report.SnapshotID}}</code>, {{.Cost.Report.UnpricedCount}} unpriced component(s))</p>
<table><tr><th>Node</th><th>Decision</th><th>Monthly amount</th><th>Reason</th></tr>
{{range .Cost.Report.Components}}<tr>
<td>{{.NodeID}}</td>
<td{{if eq (print .Decision) "cost_unknown"}} class="not-assessable"{{end}}>{{.Decision}}</td>
<td>{{if eq (print .Decision) "priced"}}{{.Currency}} {{.MonthlyAmount}}{{end}}</td>
<td class="reason">{{.Reason}}</td>
</tr>{{end}}</table>
{{if .Cost.UsageBasedCharges}}<h3>Usage-based charges</h3>
<table><tr><th>Journey</th><th>Kind</th><th>Decision</th><th>Monthly amount</th><th>Reason</th></tr>
{{range .Cost.UsageBasedCharges}}<tr>
<td>{{.JourneyID}}</td><td>{{.Kind}}</td>
<td{{if eq (print .Decision) "cost_unknown"}} class="not-assessable"{{end}}>{{.Decision}}</td>
<td>{{if eq (print .Decision) "priced"}}{{.Currency}} {{.MonthlyAmount}}{{end}}</td>
<td class="reason">{{.Reason}}</td>
</tr>{{end}}</table>{{end}}
{{else}}<p class="reason not-assessable">Not available: {{.Cost.UnavailableReason}}</p>{{end}}

<h2>Change since previous version</h2>
{{if .Delta}}<table><tr><th>Finding</th><th>Kind</th><th>Old status</th><th>New status</th><th>Reason</th></tr>
{{range .Delta}}<tr><td><code>{{.FindingID}}</code></td><td>{{.Kind}}</td><td>{{.OldStatus}}</td><td>{{.NewStatus}}</td><td class="reason">{{.Reason}}</td></tr>{{end}}
</table>{{else}}<p>No previous version to compare against, or nothing changed.</p>{{end}}

<h2>Assumptions and provenance appendix</h2>
<p>Every value in this report tagged <code>assumed</code> or <code>stated</code> — the concrete payoff of provenance-tagged assertions (I2). Empty means every computed value in this report's own findings/compliance/cost/NFR sections is a derived fact today.</p>
{{if .Assumptions}}<table><tr><th>Kind</th><th>Source</th><th>Reason</th></tr>
{{range .Assumptions}}<tr><td>{{.Kind}}</td><td>{{.Source}}</td><td class="reason">{{.Reason}}</td></tr>{{end}}
</table>{{else}}<p><em>No assumed or stated values are present in this report's own computed sections.</em></p>{{end}}

</body>
</html>
`

var reportHTMLTmpl = template.Must(template.New("report").Parse(reportHTMLTemplate))

// RenderReportHTML renders r to a single, self-contained HTML page — pure, no I/O,
// deterministic for the same Report value (see this file's own doc comment for why
// map ranges don't break that guarantee). The one field NOT escaped by html/template
// is Graph (core.Report.Graph), deliberately: it is real SVG markup meant to render
// as a diagram, not display as literal text — see the explicit template.HTML cast
// below. Every other field is escaped normally by html/template, including
// architect-entered free text (Finding titles, rationale strings, etc.), so this
// function is safe against XSS from any stated/derived string content in the report.
func RenderReportHTML(r Report) (string, error) {
	data := struct {
		Report
		Graph template.HTML
	}{Report: r, Graph: template.HTML(r.Graph)} //nolint:gosec // Graph is server-generated SVG (render.SVG), never architect-supplied text
	var buf bytes.Buffer
	if err := reportHTMLTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
