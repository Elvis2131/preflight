// This file is PC-133: the IAM policy model in the IR — principals (identity nodes,
// already modelled since PC-13/78 as NodeTypeIdentity), identity policies, resource
// policies, and role trust policies. Scenario-driven minimalism, per the Card's own
// instruction: this models what a real evaluation use case needs ("can this role read
// this bucket/table/secret") — not the entire IAM grammar.
//
// MODELLED SCOPE, stated explicitly (the Card's own acceptance criterion):
//   - Effect, Action/NotAction, Resource/NotResource, Principal, Condition — stored
//     exactly as declared (wildcards, condition keys, and Principal's several real
//     shapes — "*", {"Service": "..."}, {"AWS": [...]} — all preserved verbatim, never
//     expanded or guessed).
//   - Identity policies (inline aws_iam_role_policy, and managed
//     aws_iam_policy + aws_iam_role_policy_attachment) attach to the identity node
//     they belong to.
//   - A trust policy (a role's own assume_role_policy) attaches to that same identity
//     node.
//   - A resource-based policy (e.g. an S3 bucket policy) attaches to the RESOURCE
//     node it is declared on, not to any identity.
//
// NOT modelled (stated, not silently absent): policy variables (${aws:username}),
// permission boundaries, service control policies (SCPs — org-level, outside any
// single Terraform bundle's own scope), IAM Access Analyzer findings, and condition
// operators beyond verbatim storage — PC-134 evaluates a limited, stated subset of
// conditions; everything else is preserved so that engine can report it as
// not_assessable rather than silently ignoring it (Condition is never dropped).
package core

// PolicyStatement is one statement inside a policy document — the real AWS IAM JSON
// shape, field-for-field, preserved verbatim. Principal and Condition are untyped
// (`any`): AWS's own real JSON allows several genuinely different shapes for each
// ("*", a string, or an object with "AWS"/"Service"/"Federated" keys for Principal;
// an arbitrarily nested object for Condition) — normalizing them into one Go type
// here would mean silently discarding whichever shape didn't fit, exactly the
// "wildcards/conditions must be represented faithfully" acceptance criterion this
// ticket exists to satisfy.
type PolicyStatement struct {
	Sid         string   `json:"sid,omitempty"`
	Effect      string   `json:"effect" validate:"required,oneof=Allow Deny" jsonschema:"required,enum=Allow,enum=Deny"`
	Principal   any      `json:"principal,omitempty" jsonschema:"description=Preserved exactly as declared — a string, \"*\", or an object with AWS/Service/Federated keys. Present on trust and resource-based policies; absent on identity policies (the role itself is the principal)."`
	Action      []string `json:"action,omitempty" jsonschema:"description=Preserved verbatim, wildcards included (e.g. \"s3:Get*\") — never expanded into a guessed list of concrete actions."`
	NotAction   []string `json:"not_action,omitempty"`
	Resource    []string `json:"resource,omitempty" jsonschema:"description=Preserved verbatim, wildcards included — never expanded."`
	NotResource []string `json:"not_resource,omitempty"`

	// Condition is stored exactly as written — PC-134 evaluates a limited, stated
	// subset; every OTHER condition key/operator is preserved here regardless, so
	// that engine can name it and report not_assessable rather than silently
	// ignoring a condition this system doesn't understand yet.
	Condition map[string]any `json:"condition,omitempty"`
}

// PolicyDocument is one policy document — an inline/managed identity policy, a trust
// policy, or a resource-based policy, all the same real shape ({Version, Statement}).
type PolicyDocument struct {
	ID         string            `json:"id" validate:"required" jsonschema:"required,minLength=1,description=The originating Terraform resource's own key (e.g. aws_iam_role_policy.app_secrets, or the role's own ID for a trust policy)."`
	Version    string            `json:"version,omitempty" jsonschema:"description=The policy document's own declared Version (e.g. \"2012-10-17\"), preserved verbatim."`
	Statements []PolicyStatement `json:"statements" validate:"dive" jsonschema:"required"`
	Provenance Provenance        `json:"provenance" validate:"required" jsonschema:"required"`
}

// Validate checks this PolicyDocument against the same struct tags
// contracts/ir.schema.json is generated from.
func (d PolicyDocument) Validate() error {
	return validate.Struct(d)
}

// SymbolicResourcePrefix marks a policy Resource element that stands for a resource declared in the bundle
// whose concrete ARN is only known after apply (PC-159): "preflight-ref:aws_s3_bucket.data", optionally followed
// by "/*" for the objects within it. It is a specific, non-wildcard target.
const SymbolicResourcePrefix = "preflight-ref:"
