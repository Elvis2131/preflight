# Compliance catalog: scope, classification and limits (PC-121)

Preflight selects PCI DSS and SOC 2 and reports, control by control, what an **architecture** can and
cannot evidence. This page states the scope so nobody reads the architectural slice as the whole.

**What is stored.** Only requirement and criterion IDs, short titles written in our own words, and a
classification. The standards are licensed and are not in the repository (the source PDFs are
git-ignored); the mappings were built by reading PCI DSS v4.0.1 and the AICPA 2017 Trust Services
Criteria (revised points of focus, 2022) locally.

## Classification

| Class | Meaning | Result it can produce |
|---|---|---|
| `assessable_from_architecture` | A deterministic check decides it | satisfied, unsatisfied, not_assessable |
| `partially_assessable` | The architecture supports it, but the full requirement also needs documentation or operating evidence | **applicable** (never satisfied), unsatisfied, not_assessable |
| `not_assessable_from_architecture` | People, process, physical or organisational: no diagram can evidence it | always not_assessable, by construction (no evaluation logic exists) |

A control whose model input is missing is `not_assessable` with the reason, never a pass or a fail (I4).

## PCI DSS v4.0.1: 19 rows

One row per architecture-relevant sub-requirement, plus one "rest of Requirement N" row per principal
requirement that has one, so the not-assessable remainder stays visible.

| ID | Class | Evidence |
|---|---|---|
| 1.4.4 | assessable | database not reachable from the internet (route evaluation) |
| 1.3.1 | partial | no security group on a database admits the world |
| 1.3.2 | partial | no security group lets a database send all protocols anywhere |
| 3.5.1.2 | partial | storage encryption is supporting evidence only (see below) |
| 4.2.1 | partial | TLS settings are not modelled: not_assessable |
| 6.4.2 | partial | an internet-facing load balancer has a web ACL attached |
| 7.2.2 | partial | no identity policy grants `*` on `*` (IAM evaluation) |
| 1, 3, 4, 6, 7 "rest" rows; 2, 5, 8, 9, 10, 11, 12 | not assessable | none |

Totals: 1 assessable, 6 partial, 12 not assessable.

## SOC 2: 38 rows

Security (CC1.1 to CC9.2, 33 criteria), Availability (A1.1 to A1.3), Confidentiality (C1.1, C1.2).
Processing Integrity and Privacy are optional categories and are not cataloged. Partial: CC6.1
(encryption), CC6.3 (least privilege), CC6.6 (database reachability), CC6.7 (transmission, not modelled),
A1.2 (declared standby). Nothing is classed assessable: SOC 2 attests to controls operating over time.
Totals: 0 assessable, 5 partial, 33 not assessable.

## Decisions worth knowing

* **Storage encryption does not satisfy PCI 3.5.1.2.** v4.0.1 accepts disk-level encryption to render
  account data unreadable only on removable media, or on non-removable media when the data is also made
  unreadable another way. Encrypted storage is therefore *applicable*; the earlier catalog's "Requirement 3:
  satisfied" overclaimed. Unencrypted storage is not a failure either (protection inside the data is invisible
  here): not_assessable. The CIS control, a different standard, still judges unencrypted storage unsatisfied.
* **No web ACL is not a failure** (6.4.2): a firewall in front of the load balancer (a content-delivery or API
  layer) is outside the model.
* **IAM controls depend on how a policy reads.** A policy with in-bundle resource references
  (`aws_s3_bucket.data.arn`, `"${...arn}/*"`) is evaluated symbolically (PC-159), so ordinary Terraform policies
  are assessable. A policy it cannot read (a variable, a data source, a resource outside the bundle, a reference
  used as a principal or condition value) and an attached AWS-managed policy are not_assessable, naming the cause.

## Revisit trigger (Design §4.6)

59 controls (19 + 38 + 2 CIS) against the ~100 OPA/Rego trigger, of which 13 carry evaluation logic; the rest
are not_assessable rows with none. A test fails at 100.
