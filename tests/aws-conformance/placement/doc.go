// Package placement holds PC-105's AWS conformance tests for VPC/subnet/AZ placement
// rules — the Architect Workspace's "real placement rules" (PC-96). Every test cites
// the official AWS documentation sentence its rule rests on (tests/aws-conformance/
// harness). Rules that could not be verified against AWS's own documentation are
// deliberately absent: see core/canvas_placement.go's own header.
package placement
