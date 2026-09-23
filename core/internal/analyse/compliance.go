// This file is PC-18's compliance rule engine — deliberately one, narrow, correct
// control for v1 rather than a broad shallow set (this ticket's own Conversation
// recommendation, consistent with the golden-architecture philosophy CLAUDE.md §14
// already states elsewhere: "modelled exceptionally, rather than broad shallow
// coverage"). Both deterministic — never touches the LLM layer (reason/ has no client
// code at all yet — ADR-005/PC-77 — so this requirement is trivially, structurally
// satisfied: there is nothing here that could call it even by accident).
//
// Same package rule as everywhere else here: no dependency on preflight/core.
package analyse

// StorageEncryptionCheck evaluates whether a database's storage is encrypted at rest,
// from an already-resolved bool (the caller's job to derive from whatever the real
// provider attribute is — this function itself takes no provider-specific input).
//
// PC-18 → PC-29 history, stated rather than silently carried forward: this was
// originally RDSStorageEncryptionCheck, taking AWS's own storage_encrypted value
// directly, with a rationale string that named that attribute literally. Verified
// before encoding (CLAUDE.md §5): Terraform AWS provider docs confirm
// storage_encrypted defaults to false when unset (verified against
// raw.githubusercontent.com/hashicorp/terraform-provider-aws, 2026-09-20). The
// original security rationale was sourced from Prowler's public
// rds_instance_storage_encrypted check metadata (PCI-DSS, AWS Foundational Security
// Best Practices, NIST 800-53/CSF, ISO 27001, HIPAA, GDPR) — a citation specific to
// that AWS check, not re-verified against an Azure-equivalent control.
//
// Generalized here (PC-29) to run against ANY managed_database node, AWS or Azure:
// the CLASSIFICATION logic (encrypted bool -> satisfied/unsatisfied/not_assessable) is
// a genuinely universal security property, independent of which cloud states it or
// what the underlying attribute is called — encrypted-at-rest data is exactly as
// protected regardless of provider. What is NOT reused across clouds: the specific
// compliance-FRAMEWORK citation above (Prowler's AWS-specific check) is not claimed to
// apply to Azure SQL's transparent_data_encryption_enabled — the rationale text below
// states the technical fact only, naming no specific attribute or framework, so it
// stays true regardless of which provider's caller supplies the bool.
//
// NAMED GAP, carried forward unchanged from PC-18: this ticket's own title says "CIS
// framework." A citable CIS AWS Foundations Benchmark control ID specifically covering
// RDS storage encryption was not found via any publicly-accessible source — CLAUDE.md
// §5: uncertain stays unknown, never guessed.
func StorageEncryptionCheck(encrypted *bool) (status string, rationale string) {
	if encrypted == nil {
		return "not_assessable", "storage encryption is not stated for this resource"
	}
	if *encrypted {
		return "satisfied", "storage encryption is enabled — data at rest, snapshots, and automated backups are protected"
	}
	return "unsatisfied", "storage encryption is not enabled — data at rest, snapshots, and automated backups remain in plaintext; an attacker with access to a copied snapshot, a compromised backup, or the underlying storage can read sensitive data, risking large-scale exfiltration"
}
