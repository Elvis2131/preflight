package server

import (
	"testing"

	"preflight/core"
)

func TestStore_StoreAndRetrieveVersion(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	if err := store.EnsureSession("sess-1"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	ir := &core.IR{SchemaVersion: "1.0.0", VersionNumber: 1, VersionHash: "sha256:test"}
	v := StoredVersion{SessionID: "sess-1", VersionNumber: 1, IR: ir, Findings: nil, Scorecard: core.Scorecard{VersionNumber: 1}}

	if err := store.StoreVersion(v); err != nil {
		t.Fatalf("StoreVersion: %v", err)
	}

	got, ok, err := store.LatestVersion("sess-1")
	if err != nil {
		t.Fatalf("LatestVersion: %v", err)
	}
	if !ok {
		t.Fatal("expected a stored version, got none")
	}
	if got.VersionNumber != 1 || got.IR.VersionHash != "sha256:test" {
		t.Fatalf("got %+v, want VersionNumber=1 IR.VersionHash=sha256:test", got)
	}
}

func TestStore_LatestVersion_NoVersionsYet(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	if err := store.EnsureSession("sess-new"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	_, ok, err := store.LatestVersion("sess-new")
	if err != nil {
		t.Fatalf("LatestVersion: %v", err)
	}
	if ok {
		t.Fatal("expected no version for a brand-new session")
	}
}

func TestStore_MultipleVersions_LatestWins(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	if err := store.EnsureSession("sess-multi"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	for i := 1; i <= 3; i++ {
		v := StoredVersion{
			SessionID: "sess-multi", VersionNumber: i,
			IR:        &core.IR{SchemaVersion: "1.0.0", VersionNumber: i, VersionHash: "sha256:v" + string(rune('0'+i))},
			Scorecard: core.Scorecard{VersionNumber: i},
		}
		if err := store.StoreVersion(v); err != nil {
			t.Fatalf("StoreVersion(%d): %v", i, err)
		}
	}

	got, ok, err := store.LatestVersion("sess-multi")
	if err != nil || !ok {
		t.Fatalf("LatestVersion: ok=%v err=%v", ok, err)
	}
	if got.VersionNumber != 3 {
		t.Fatalf("VersionNumber = %d, want 3 (the latest)", got.VersionNumber)
	}

	v1, ok, err := store.GetVersion("sess-multi", 1)
	if err != nil || !ok {
		t.Fatalf("GetVersion(1): ok=%v err=%v", ok, err)
	}
	if v1.VersionNumber != 1 {
		t.Fatalf("GetVersion(1).VersionNumber = %d, want 1", v1.VersionNumber)
	}
}

func TestStore_EnsureSession_Idempotent(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	for i := 0; i < 3; i++ {
		if err := store.EnsureSession("sess-idem"); err != nil {
			t.Fatalf("EnsureSession call %d: %v", i, err)
		}
	}
}
