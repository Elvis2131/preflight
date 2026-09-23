// Package validate is the validation-ladder harness (P3, credentialed): Rung 1
// (Toxiproxy/kind topology replica), Rung 2 (LocalStack), Rung 3 (ephemeral
// apply/experiment/destroy against a real cloud account). PC-24 and PC-25 build the
// real harness. This is the only package tree runnerd (cmd/runnerd) is permitted to
// call into for anything credentialed (ADR-003).
package validate
