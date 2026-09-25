// Package creds is PC-10's remaining acceptance criterion, resolved: "P1 has zero
// imports of credential-handling code, verified by Go's internal/ package placement
// (compiler-enforced) — not by import-linter." PC-10's own comment history had left
// this honestly incomplete ("not violated because not built" — no credential-handling
// code existed anywhere to import, so the criterion was vacuous rather than actively
// enforced) and the gap was recorded in ADR-003 itself rather than smoothed over:
// "nothing yet enforces P3 being the only credential holder — needs a check, not just
// discipline."
//
// This package IS that check, standing up the boundary before the content that needs
// it exists — exactly the sequencing cmd/runnerd/main.go's own comment already
// describes for the P3 process itself ("the boundary is structural before there is
// anything inside it worth protecting"). Go's internal/ visibility rule means a
// package at cmd/runnerd/internal/... is importable only by code rooted at
// cmd/runnerd/ — compiler-enforced, the same mechanism (and the same package name,
// deliberately) as core/internal/{ir,analyse,simulate}'s existing I1 boundary
// (core/boundary_test.go). See cmd/runnerd/internal/creds/boundary_test.go for the
// proof, built with that exact same synthesize-and-build technique: a scratch package
// outside cmd/runnerd/ that tries to import this one, confirmed rejected by `go build`
// itself, not merely asserted in a comment.
//
// Deliberately empty of any real credential-loading logic: none exists yet (Rung 2/3 of
// validate/, PC-25, haven't been built). When that logic is written, it belongs here —
// or in a further subpackage of cmd/runnerd/internal/ — not in bare validate/, which
// (unlike this package) carries no compiler-enforced restriction on who can import it.
// validate/'s own Rung 1 harness (PC-24) holds no cloud credentials at all (Docker/
// Toxiproxy only) and correctly lives outside this boundary; a future Rung 2/3 addition
// that DOES load real cloud credentials should move that specific code under this tree,
// not add it to validate/ directly.
package creds
