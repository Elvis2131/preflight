// Package providers holds cloud provider mappings as data, not code (CLAUDE.md §6):
// aws/ and azure/ capability models mapping cloud-specific resources onto the canonical
// IR (core/internal/ir). PC-13 (AWS) and PC-22 (Azure) build the real content; this file
// exists only to make the package non-empty until then.
package providers
