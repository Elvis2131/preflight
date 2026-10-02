package rung3

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

func ptr(f float64) *float64 { return &f }

// Analyse turns the raw timeline into the measured numbers. It is pure: same timeline, same result.
// A probe with AtMs < 0 happened before the fault was started (the baseline).
func Analyse(probes []Probe, health []HealthSample, faultState string) Observed {
	o := Observed{FaultState: faultState, BaselineServedBy: []string{}, ServedByAfterLastFail: []string{}}
	baseline := map[string]bool{}
	var firstFail, lastFail int64 = -1, -1
	for _, p := range probes {
		if p.AtMs < 0 {
			o.BaselineProbes++
			if p.Failed() {
				o.BaselineFailures++
			} else {
				baseline[p.Body] = true
			}
			continue
		}
		o.ProbesAfterFault++
		if p.Failed() {
			o.FailuresAfterFault++
			if firstFail < 0 {
				firstFail = p.AtMs
			}
			lastFail = p.AtMs
		}
	}
	o.BaselineServedBy = sortedKeys(baseline)
	if firstFail >= 0 {
		o.FirstFailureSeconds = ptr(float64(firstFail) / 1000)
		o.LastFailureSeconds = ptr(float64(lastFail) / 1000)
		o.FailureWindowSeconds = ptr(float64(lastFail-firstFail) / 1000)
		inWindow, failed := 0, 0
		for _, p := range probes {
			if p.AtMs >= firstFail && p.AtMs <= lastFail {
				inWindow++
				if p.Failed() {
					failed++
				}
			}
		}
		if inWindow > 0 {
			o.FailureShareInWindow = ptr(float64(failed) / float64(inWindow))
		}
	}
	after := map[string]bool{}
	for _, p := range probes {
		if p.AtMs > lastFail && p.AtMs >= 0 && !p.Failed() {
			after[p.Body] = true
		}
	}
	o.ServedByAfterLastFail = sortedKeys(after)
	for _, h := range health {
		if h.AtMs < 0 {
			continue
		}
		if t, ok := h.Targets["web-a"]; ok && t.State != "healthy" && o.WebAFirstNotHealthySec == nil {
			o.WebAFirstNotHealthySec = ptr(float64(h.AtMs) / 1000)
		}
	}
	if len(health) > 0 {
		last := health[len(health)-1].Targets["web-a"]
		o.WebAFinalState, o.WebAFinalReason = last.State, last.Reason
	}
	return o
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// predicted is the part of validate/rung3/predictions.json this package reads.
type predicted struct {
	Statements []struct {
		ID    string `json:"id"`
		Claim string `json:"claim"`
	} `json:"statements"`
}

// detectionBoundSeconds is P3's bound: unhealthy_threshold x interval + one interval of phase,
// read from the same constants validate/rung3 derived the prediction from.
const detectionBoundSeconds = 30

// Compare checks each committed prediction against what was measured. It never edits a prediction.
func Compare(predictionsPath string, probes []Probe, o Observed) ([]Comparison, error) {
	b, err := os.ReadFile(predictionsPath)
	if err != nil {
		return nil, err
	}
	var p predicted
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	var out []Comparison
	for _, s := range p.Statements {
		c := Comparison{ID: s.ID, Claim: s.Claim}
		switch s.ID {
		case "P1":
			c.Verdict, c.Evidence = judgeP1(probes, o)
		case "P3":
			c.Verdict, c.Evidence = judgeP3(o)
		case "P4":
			if o.WebAFinalState == "unused" && o.WebAFinalReason == "Target.InvalidState" {
				c.Verdict, c.Evidence = "confirmed", "web-a ended unused / Target.InvalidState"
			} else {
				c.Verdict, c.Evidence = "refuted", fmt.Sprintf("web-a ended %q / %q", o.WebAFinalState, o.WebAFinalReason)
			}
		default:
			c.Verdict, c.Evidence = "not_observed", "the run does not test this claim"
		}
		out = append(out, c)
	}
	return out, nil
}

// judgeP1: after the detection window every answer is a 200 from web-b. "After" is the second half
// of the watched window, at most its last 30 s, with at least 20 requests, so a short window cannot
// pass by having no data.
func judgeP1(probes []Probe, o Observed) (string, string) {
	var maxAt int64
	for _, p := range probes {
		if p.AtMs > maxAt {
			maxAt = p.AtMs
		}
	}
	tail := maxAt / 2
	if tail > 30_000 {
		tail = 30_000
	}
	from := maxAt - tail
	n, bad := 0, 0
	for _, p := range probes {
		if p.AtMs < from || p.AtMs < 0 {
			continue
		}
		n++
		if p.Failed() || p.Body != "web-b" {
			bad++
		}
	}
	switch {
	case n < 20:
		return "refuted", fmt.Sprintf("only %d requests in the final %d s; too few to confirm", n, tail/1000)
	case bad > 0:
		return "refuted", fmt.Sprintf("%d of %d requests in the final %d s were not a 200 from web-b", bad, n, tail/1000)
	}
	return "confirmed", fmt.Sprintf("all %d requests in the final %d s were 200 from web-b (baseline served by %v)", n, tail/1000, o.BaselineServedBy)
}

func judgeP3(o Observed) (string, string) {
	if o.FailureWindowSeconds == nil {
		return "refuted", "no request failed at all, so the claim that some requests fail until the ALB detects the loss did not hold"
	}
	w, share := *o.FailureWindowSeconds, 0.0
	if o.FailureShareInWindow != nil {
		share = *o.FailureShareInWindow
	}
	ev := fmt.Sprintf("failures spanned %.1f s (bound %d s); %.0f%% of requests in that window failed (predicted about half)", w, detectionBoundSeconds, share*100)
	if w > detectionBoundSeconds {
		return "refuted", ev + ": the window is longer than the bound"
	}
	if share < 0.25 || share > 0.75 {
		return "refuted", ev + ": the failure share is not about half"
	}
	return "confirmed", ev
}
