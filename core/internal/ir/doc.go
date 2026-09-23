// Package ir is a placeholder for the two-level IR (canonical semantic model + provider
// capability model, resolution states) that PC-11 designs and PC-7 freezes as
// contracts/ir.schema.json. It exists now only so the core/internal/ boundary (I1) has a
// real package to protect from day one, rather than being introduced later as an
// afterthought alongside the first real IR type.
//
// This package is compiler-private to code rooted under core/ (Go's internal/
// visibility rule) — see core/boundary_test.go for a runtime-verified proof of that,
// not just an assertion of it.
package ir

// Placeholder is a stand-in for the real IR node/edge types. It exists only so this
// package is non-empty; nothing about its shape should be read as a decision about the
// real IR. Delete it the moment PC-11 lands real types.
type Placeholder struct{}
