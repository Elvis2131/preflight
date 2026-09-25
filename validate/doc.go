// Package validate is the validation-ladder harness: Rung 1 (Toxiproxy/kind topology
// replica, PC-24, rung1.go — zero cloud credentials), Rung 2 (LocalStack), Rung 3
// (ephemeral apply/experiment/destroy against a real cloud account, P3, credentialed —
// PC-25). This is the only package tree runnerd (cmd/runnerd) is permitted to call
// into for anything credentialed (ADR-003).
package validate
