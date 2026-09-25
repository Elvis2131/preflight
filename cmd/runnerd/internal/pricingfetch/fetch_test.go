package pricingfetch

// PC-116's own acceptance criterion: "Fetcher produces a snapshot for the agreed
// services/regions; normalised table checked in as a small test fixture." The two
// fixtures here (testdata/awselb_region_index.json, testdata/awselb_useast1_offer.json)
// are REAL data — fetched verbatim, live, from AWS's own Bulk Price List API
// (pricing.us-east-1.amazonaws.com) on 2026-09-25, not hand-invented — so this proves
// the parser against a real, checked-in AWS response shape without a network call in
// the normal test run (this codebase's own established discipline: an external
// dependency is verified periodically/deliberately, e.g. cmd/check-conformance-links,
// never on every commit — see fetch_live_test.go for the deliberately separate,
// build-tag-gated live-network proof).

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type fakeHTTPClient struct {
	byURLSuffix map[string]string // URL suffix -> testdata file path
}

func (f fakeHTTPClient) Get(url string) (*http.Response, error) {
	for suffix, path := range f.byURLSuffix {
		if strings.HasSuffix(url, suffix) {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data))}, nil
		}
	}
	return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewReader(nil))}, nil
}

func awselbFakeClient() fakeHTTPClient {
	return fakeHTTPClient{byURLSuffix: map[string]string{
		"AWSELB/current/region_index.json": "testdata/awselb_region_index.json",
	}}
}

func TestFetch_AWSELB_RealFixture(t *testing.T) {
	client := awselbFakeClient()
	// The region index's own currentVersionUrl for us-east-1 is a REAL, dated path
	// (fetched live 2026-09-25) — read it back out so the fake client also serves the
	// offer file itself at exactly the URL Fetch will actually request.
	idx, err := fetchRegionIndex(client, "AWSELB")
	if err != nil {
		t.Fatalf("fetchRegionIndex: %v", err)
	}
	offerURL, ok := idx.Regions["us-east-1"]
	if !ok {
		t.Fatal("fixture has no us-east-1 region entry")
	}
	client.byURLSuffix[offerURL.CurrentVersionURL] = "testdata/awselb_useast1_offer.json"

	entries, source, err := Fetch(client, "AWSELB", "us-east-1")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one normalized price entry from the real fixture")
	}
	if !strings.Contains(source, "AWSELB") {
		t.Errorf("source = %q, want it to name AWSELB", source)
	}
	for _, e := range entries {
		if e.Service != "AWSELB" || e.Region != "us-east-1" || e.Currency != "USD" {
			t.Errorf("entry has wrong Service/Region/Currency: %+v", e)
		}
		if e.Price < 0 {
			// A real $0.0 entry (e.g. AWS Outposts LCU usage) is legitimate AWS data,
			// not a parse error — only a negative price would be wrong.
			t.Errorf("entry has a negative price: %+v", e)
		}
		if e.Unit == "" {
			t.Errorf("entry has no unit: %+v", e)
		}
	}
}

func TestFetchSnapshot_ProducesADatedSnapshot(t *testing.T) {
	client := awselbFakeClient()
	idx, err := fetchRegionIndex(client, "AWSELB")
	if err != nil {
		t.Fatalf("fetchRegionIndex: %v", err)
	}
	client.byURLSuffix[idx.Regions["us-east-1"].CurrentVersionURL] = "testdata/awselb_useast1_offer.json"

	snap, err := FetchSnapshot(client, "snap-1", []string{"AWSELB"}, "us-east-1")
	if err != nil {
		t.Fatalf("FetchSnapshot: %v", err)
	}
	if snap.ID != "snap-1" {
		t.Errorf("ID = %q, want snap-1", snap.ID)
	}
	if snap.FetchedAt.IsZero() {
		t.Error("FetchedAt is zero, want a real dated timestamp")
	}
	if snap.Disclaimer == "" {
		t.Error("Disclaimer is empty — AWS's own disclaimer must always be carried")
	}
	if len(snap.Entries) == 0 {
		t.Fatal("expected real entries in the snapshot")
	}
}

func TestFetch_UnknownRegion_RealError(t *testing.T) {
	client := awselbFakeClient()
	_, _, err := Fetch(client, "AWSELB", "mars-central-1")
	if err == nil {
		t.Fatal("expected an error for a region absent from the real fixture")
	}
}
