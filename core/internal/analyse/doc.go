// Package analyse is SPOF/min-cut, reachability, capacity, and the compliance rule
// engine (CLAUDE.md §6). resolution.go's AssessNode/AssessEdge (PC-11) are its first
// real content — the foundational resolution-state gate every other analysis in this
// package builds on. The min-cut implementation (PC-14, a named cost of ADR-004) and
// the compliance rule engine (PC-18) are not yet built.
package analyse
