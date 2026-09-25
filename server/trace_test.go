package server_test

// PC-114: proves server.Trace — the actual production code path — works against a
// real assessed version's stored IR, not just core.BuildTrace in isolation
// (core/trace_test.go covers that with a hand-built synthetic scenario).

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"preflight/server"
)

func TestTrace_AgainstARealAssessedVersion(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	assessResp, err := server.Assess(store, server.AssessRequest{
		SessionID:    "trace-test",
		BundleDir:    bundleDir,
		WorkloadPath: workloadPath,
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}

	tr, err := server.Trace(store, server.TraceRequest{
		SessionID:     "trace-test",
		VersionNumber: assessResp.VersionNumber,
		Source:        "aws_eks_cluster.payments",
		Destination:   "aws_db_instance.payments",
		Protocol:      "tcp",
		Port:          5432,
	})
	if err != nil {
		t.Fatalf("Trace: %v", err)
	}
	if len(tr.Steps) == 0 {
		t.Fatal("expected a real, non-empty step list against the golden AWS bundle")
	}
	if tr.Steps[0].Step != "resolve_destination" {
		t.Errorf("first step = %q, want resolve_destination", tr.Steps[0].Step)
	}
}

func TestTrace_InternetOriginated(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	assessResp, err := server.Assess(store, server.AssessRequest{
		SessionID:    "trace-test-internet",
		BundleDir:    bundleDir,
		WorkloadPath: workloadPath,
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}

	tr, err := server.Trace(store, server.TraceRequest{
		SessionID:     "trace-test-internet",
		VersionNumber: assessResp.VersionNumber,
		Destination:   "aws_lb.payments",
		SourceCIDR:    "0.0.0.0/0",
		Protocol:      "tcp",
		Port:          443,
	})
	if err != nil {
		t.Fatalf("Trace: %v", err)
	}
	for _, s := range tr.Steps {
		if s.Step == "resolve_source" {
			t.Errorf("got a resolve_source step for an internet-originated request, want none")
		}
	}
}

func TestTrace_MissingDestination_Returns400(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	_, err = server.Trace(store, server.TraceRequest{
		SessionID: "whatever", VersionNumber: 1, Protocol: "tcp",
	})
	var apiErr *server.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a *server.APIError", err)
	}
	if apiErr.Code != "invalid_request_body" {
		t.Errorf("Code = %q, want invalid_request_body", apiErr.Code)
	}
	if apiErr.Status != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", apiErr.Status, http.StatusBadRequest)
	}
}

func TestTrace_UnknownSession_Returns404SessionNotFound(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	_, err = server.Trace(store, server.TraceRequest{
		SessionID: "never-assessed", VersionNumber: 1,
		Destination: "aws_db_instance.payments", Protocol: "tcp", Port: 5432,
	})
	var apiErr *server.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a *server.APIError", err)
	}
	if apiErr.Code != "session_not_found" {
		t.Errorf("Code = %q, want session_not_found", apiErr.Code)
	}
	if apiErr.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", apiErr.Status, http.StatusNotFound)
	}
}

func TestTrace_SessionExistsButVersionDoesnt_Returns404VersionNotFound(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Assess(store, server.AssessRequest{
		SessionID: "trace-real-session-wrong-version", BundleDir: bundleDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess: %v", err)
	}

	_, err = server.Trace(store, server.TraceRequest{
		SessionID: "trace-real-session-wrong-version", VersionNumber: 99,
		Destination: "aws_db_instance.payments", Protocol: "tcp", Port: 5432,
	})
	var apiErr *server.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a *server.APIError", err)
	}
	if apiErr.Code != "version_not_found" {
		t.Errorf("Code = %q, want version_not_found — the session itself DOES exist", apiErr.Code)
	}
	if apiErr.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", apiErr.Status, http.StatusNotFound)
	}
}
