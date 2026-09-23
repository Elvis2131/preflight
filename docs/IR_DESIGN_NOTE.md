# Designing an IR that refuses to guess

*A design note on Preflight's intermediate representation — PC-16.*

## What this is, and why it exists

Preflight is an architecture assurance tool: point it at a Terraform bundle and a
declared workload (target availability, RPO/RTO, compliance profile), and it tells you
what actually happens when a piece of that architecture fails — not by asking an LLM to
guess, but by parsing the real infrastructure-as-code into a structured model and
running deterministic graph and compliance checks over it.

That structured model — the **Intermediate Representation (IR)** — is the thing this
note is about. Every claim Preflight ever makes about an architecture passes through
it, so its design choices are the load-bearing ones in the whole system. This note
covers four of them: how the IR separates a provider's specific mechanics from its
canonical shape, how it represents "we don't fully know," how it keeps redundancy from
being mistaken for capacity, and how it tracks where every value came from. It should
be readable on its own — no other Preflight document required.

## Choice 1: two levels, not one — canonical structure and provider capability

The obvious way to represent "a Terraform bundle" is one big tree that mirrors the
HCL: here's an `aws_db_instance`, here are its forty-odd attributes. Preflight doesn't
do that. It splits every resource into two layers:

- **Canonical structure** — node types, edges, topology, placement, redundancy: the
  shape a failure-mode or compliance check actually reasons over. An AWS RDS instance
  and an Azure SQL database are both, canonically, a `managed_database` node.
- **Capability** — the small set of typed, per-node facts that failure and compliance
  semantics actually *depend on*: replication mode, failover mechanism, encryption
  mechanism, multi-AZ implementation. Everything else about the resource is retained
  verbatim for attribute-level lookups, but isn't part of the canonical model.

The reason for the split, not just its existence: a check written against "does this
node have synchronous replication" should never need to know whether that fact came
from AWS's `multi_az` boolean or Azure's `zone_redundant` boolean. Write the check once
against the canonical `replication_mode` field, and it works on both clouds — which is
the whole cloud-agnostic claim this project makes, made checkable rather than asserted.

**Working through this against real Terraform surfaced one case where the split
doesn't hold cleanly: a Web Application Firewall (WAF).**

A WAF has its own identity, its own configuration (managed rule groups) — and,
critically, a WAF policy can be perfectly configured and simply never attached to the
load balancer it's supposed to protect. That's not a capability fact *about* the load
balancer (a `waf_attached: bool` flag on the load balancer would hide exactly the
structural fact worth catching — a missing edge — not express it). A missing
attachment is a real, independently-failable architectural defect, the same shape as
a firewall rule nobody remembered to apply. It needs to be its own node with its own
edge to what it protects, not a footnote on another node's capability list.

The resolution: model a WAF as a `network_boundary` node (the same canonical bucket
already used for NAT gateways, internet gateways, and subnets — a WAF is a
network-perimeter control, which is exactly what that bucket already means),
connected to the load balancer it protects by a `depends_on` edge — the load
balancer's protection posture *depends on* the attachment holding; `routes_to` would
misdescribe what a WAF does to a request, since it isn't a data-plane hop.

This is a deliberate interpretation of an already-fixed vocabulary, not a change to
it. If a future service (GuardDuty, Shield, a dedicated firewall) also doesn't fit
either existing bucket, that's a real signal the vocabulary itself needs to grow — a
versioned schema change, not something to paper over silently with another borrowed
label.

## Choice 2: three resolution states, flat, no sub-states

Terraform lets you reference things you haven't fully declared — a `count` loop over
an unknown-at-parse-time list, a security group referencing a rule that lives in a
module the parser never saw. The IR needs an honest way to represent "I don't fully
know what this is" that survives all the way to the final output, rather than silently
becoming a wrong answer three functions later.

Every node and edge in the IR carries exactly one of three resolution states:

- **known** — fully declared, no missing pieces.
- **inferred** — resolved from a reference the engine could actually follow (e.g. a
  security group ID that traces to a real, parsed resource).
- **unresolved** — references something not present in the bundle at all (a dangling
  reference, or a construct outside the parser's supported subset).

No finer subdivision. This was a real design question — should "a dangling reference"
and "a construct the parser can't handle at all" be distinguished at the IR level? —
and the answer worked out to no: by the time a node reaches the IR, "the engine cannot
determine this with confidence" is one fact every downstream check needs to act on
identically, regardless of *why* it's true. The parser's own diagnostic output can and
does distinguish the underlying reasons (that's useful for a human debugging their
Terraform); the IR itself only needs the coarser fact, because nothing downstream
treats the two reasons differently.

**The propagation guarantee is enforced structurally, not by convention.** The rule —
"a node with an unresolved reference must never produce a pass/fail finding, only
`not_assessable`" — isn't a rule analysis authors are trusted to remember. Every
analysis function reaches a node or edge's value through exactly one gate, which
checks the resolution state first and returns `not_assessable` automatically before
the caller's own logic ever runs. A future author writing a new compliance check
physically cannot skip this step, because there is no other path to the value at all.

## Choice 3: capacity is declared, never counted

A subtler failure mode this IR is designed to prevent: conflating "how many copies of
something survived a failure" with "how much load those survivors can carry." They
sound similar and are not.

Instance count is a **redundancy fact** — fully derivable from the graph. If a failure
kills one availability zone, you can count exactly how many instances of a node type
remain, with certainty. Load capacity per instance is a **declaration** someone made
about the real world — how much traffic one instance can actually serve — and no
amount of graph analysis can conjure that number from topology alone.

The IR keeps these strictly separate. A node type's per-instance capacity is only ever
read from an explicit declaration (the workload's own stated NFRs); if no such
declaration exists, the honest answer is `capacity_unknown` — never zero, never an
assumption, and never "well, there are 3 instances, so probably fine." Treating
instance count as if it were capacity is a natural, easy-to-make mistake — natural
enough that it produced a real, documented defect in an earlier reference
implementation this project studied — which is exactly why the two computations are
kept in separate code paths that can't be silently merged by an incremental edit later.

## Choice 4: every value carries where it came from

The last piece: nothing in the IR — no field, no derived finding, no compliance
verdict — exists without a **provenance tag** stating how it was established. Five
kinds, applied uniformly:

- **stated** — read directly from the workload declaration or the Terraform itself.
- **derived** — a deterministic computation over already-known IR facts (a graph
  traversal, a containment check).
- **assumed** — a documented provider default applied because the user left a field
  unset, always paired with what the override would look like.
- **llm_reasoned** — contextual judgment from a language model, always required to
  cite the specific IR evidence it reasoned from.
- **observed** — measured in a real, executed experiment, not predicted.

The schema makes an untagged assertion unrepresentable — there's no code path that
produces a bare value with no provenance attached. This matters most at the boundary
between what the tool *knows* and what it's *guessing*: a value tagged `assumed`
(AWS's own documented default for a field the user never set) and a value tagged
`derived` (something the engine computed with certainty from what's actually declared)
can look identical printed on a screen, but they are different claims, and a reviewer
deciding whether to trust a finding needs to be able to tell them apart without asking
the tool's author.

## Why these four choices are one decision, not four

Read together, the pattern is the same rule applied at four different layers: **never
let a "we don't actually know" collapse into a value that looks like an answer.**
Two-level modeling keeps a provider's specific mechanism from being silently discarded
when it doesn't fit a canonical slot. Resolution states keep a partially-declared
Terraform bundle from producing confident nonsense. Capacity separation keeps
redundancy from being mistaken for headroom. Provenance keeps an assumption from
being mistaken for a fact. None of these are exotic ideas; what makes them load-
bearing here is that each one is enforced by the type system and the call graph, not
by a convention a future contributor has to remember.
