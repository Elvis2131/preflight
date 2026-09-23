// Package ui is the graph/diff/timeline frontend. Reads the server API only — no AWS
// semantics live here (mirrors the "UI-owned AWS semantics" anti-pattern this project
// already names and rejects elsewhere). Built last, per CLAUDE.md §21: after the
// agent-iteration acceptance test (PC-28) proves the core loop works headlessly.
package ui
