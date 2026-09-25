package core

import "time"

// ADR is a decision record, one of PRD §4's two first-class lifecycle objects:
// "versioning records what changed; ADRs record why — decision, reason, trade-offs,
// controls satisfied, failure modes addressed (linked by ID)." This is Preflight's own
// product-level ADR object, distinct from (though the same shape of idea as) the
// project's own docs/adr/ engineering decision log — this one is emitted BY the product
// about a user's architecture, not authored by this project's maintainers about
// Preflight itself.
type ADR struct {
	ID       string `json:"id" validate:"required" jsonschema:"required,minLength=1"`
	Decision string `json:"decision" validate:"required" jsonschema:"required,minLength=1"`
	Reason   string `json:"reason" validate:"required" jsonschema:"required,minLength=1"`

	TradeOffs []string `json:"trade_offs,omitempty"`

	// ControlsSatisfied and FailureModesAddressed are "linked by ID" per PRD §4 — IDs
	// referencing objects in other contracts (a compliance control ID, a Finding.ID)
	// rather than embedded copies, so the two never drift out of sync with each other.
	ControlsSatisfied     []string `json:"controls_satisfied,omitempty"`
	FailureModesAddressed []string `json:"failure_modes_addressed,omitempty"`

	Provenance Provenance `json:"provenance" validate:"required" jsonschema:"required"`
}

// Validate checks this ADR against the same struct tags contracts/adr.schema.json is
// generated from.
func (a ADR) Validate() error {
	return validate.Struct(a)
}

// Waiver is PRD §4's second lifecycle object: "an evidence object: who accepted which
// finding, when, why, against which version, with optional expiry." PRD §4 is explicit
// about scope: "v1 defines the schema and displays waived findings as accepted;
// enforcement (auto-expiry on material change) is P2" — this ticket (PC-20) builds
// exactly that v1 half, via Applies/ApplyWaivers below; the P2 half (auto-invalidating
// a waiver when the finding it names has materially changed since AgainstVersion,
// which needs a real "did this finding change" diff engine — a separate, larger piece
// of work) is not built here.
type Waiver struct {
	AcceptedBy string `json:"accepted_by" validate:"required" jsonschema:"required,minLength=1"`
	FindingID  string `json:"finding_id" validate:"required" jsonschema:"required,minLength=1,description=Linked by ID, per PRD §4 — not an embedded Finding copy."`
	AcceptedAt string `json:"accepted_at" validate:"required" jsonschema:"required,format=date-time"`
	Reason     string `json:"reason" validate:"required" jsonschema:"required,minLength=1"`

	// AgainstVersion is the IR VersionNumber this waiver was accepted against — PRD §4:
	// "against which version" — recorded so a later reviewer can see exactly what the
	// finding looked like when someone signed off on it. It is a FLOOR for display, not
	// an exact-match gate: Applies treats the waiver as still in effect for any later
	// version too (versionNumber >= AgainstVersion), because the engineer's whole
	// stated reason for wanting this object ("stops reappearing as noise on every
	// iteration") would otherwise be defeated the moment the very next /assess call
	// bumped VersionNumber by one, as it does on every iteration of this project's own
	// author/evaluate/modify/re-evaluate loop. What a same-version-only reading would
	// actually buy — catching a waiver that's gone stale because the finding changed
	// underneath it — is precisely P2's auto-expiry-on-material-change job, not this
	// field's; until that engine exists, only ExpiresAt (below) or a fresh waiver
	// record can end a waiver early.
	AgainstVersion int `json:"against_version" validate:"gte=1" jsonschema:"required,minimum=1"`

	// ExpiresAt is optional (PRD §4: "with optional expiry"). A nil value means no
	// stated expiry, not an error and not "never expires" as an asserted fact — it is
	// simply absent, consistent with this schema's general rule that absence is not a
	// substitute answer for a value that was never given.
	ExpiresAt *string `json:"expires_at,omitempty" jsonschema:"format=date-time"`
}

// Validate checks this Waiver against the same struct tags contracts/adr.schema.json is
// generated from (Design §5: ADR & waiver share one file, adr.schema.json).
func (w Waiver) Validate() error {
	return validate.Struct(w)
}

// Applies reports whether this waiver is in effect at versionNumber and now — the
// exact rule ApplyWaivers (core/scorecard.go) uses to decide which scorecard entries
// display as "accepted". See AgainstVersion's own doc comment above for why this is
// versionNumber >= AgainstVersion rather than an exact match. An ExpiresAt that fails
// to parse as RFC3339 is treated as expired, not as "no expiry" — a stated-but-
// unreadable expiry is not an honest basis for "still valid."
func (w Waiver) Applies(versionNumber int, now time.Time) bool {
	if versionNumber < w.AgainstVersion {
		return false
	}
	if w.ExpiresAt == nil {
		return true
	}
	expires, err := time.Parse(time.RFC3339, *w.ExpiresAt)
	if err != nil {
		return false
	}
	return now.Before(expires)
}
