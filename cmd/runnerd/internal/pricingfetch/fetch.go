// Package pricingfetch is PC-116/ADR-006's real AWS Bulk Price List API fetcher —
// compiler-private to cmd/runnerd (Go's internal/ visibility rule), the same
// structural guarantee cmd/runnerd/internal/creds already established for PC-10,
// extended here to cover any outbound network call (ADR-006 §2), not just a
// credentialed one. cmd/assessd/cmd/reasond have no import path to this package at
// all — proven by cmd/runnerd/internal/creds/boundary_test.go's own
// TestP1AndP2HaveZeroCloudSDKOrValidateImports, extended to also forbid this package.
//
// No AWS credentials are used or needed: the Bulk API is plain, unauthenticated
// HTTPS GET (verified live, 2026-09-25 — see ADR-006 §1). This package still lives
// here, not in pricing/ itself, because the boundary that matters is "no network call
// on the assessment path" (NFR-1/I1), not "no credentialed call."
package pricingfetch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"preflight/pricing"
)

const bulkAPIBase = "https://pricing.us-east-1.amazonaws.com"

// regionIndex is the real shape of .../<service>/current/region_index.json —
// verified live against AmazonElastiCache, 2026-09-25 (ADR-006 §1).
type regionIndex struct {
	Regions map[string]struct {
		RegionCode        string `json:"regionCode"`
		CurrentVersionURL string `json:"currentVersionUrl"`
	} `json:"regions"`
}

// offerFile is the real shape of one service/region's own price list file —
// verified live against AWSELB/us-east-1, 2026-09-25 (ADR-006 §1).
type offerFile struct {
	Disclaimer string `json:"disclaimer"`
	Version    string `json:"version"`
	Products   map[string]struct {
		SKU           string            `json:"sku"`
		ProductFamily string            `json:"productFamily"`
		Attributes    map[string]string `json:"attributes"`
	} `json:"products"`
	Terms struct {
		OnDemand map[string]map[string]struct {
			PriceDimensions map[string]struct {
				Description  string            `json:"description"`
				BeginRange   string            `json:"beginRange"`
				EndRange     string            `json:"endRange"`
				Unit         string            `json:"unit"`
				PricePerUnit map[string]string `json:"pricePerUnit"`
			} `json:"priceDimensions"`
		} `json:"OnDemand"`
	} `json:"terms"`
}

// httpClient is a minimal interface so tests can substitute a fake without a real
// network call — the real fetcher (used by cmd/runnerd's own CLI) passes
// http.DefaultClient.
type httpClient interface {
	Get(url string) (*http.Response, error)
}

// Fetch retrieves one service's real, current on-demand price list for one region
// from the live AWS Bulk API and normalizes it into pricing.PriceEntry rows — ADR-006
// §1/§3/§4. serviceCode is AWS's own offer code (e.g. "AmazonRDS", "AWSELB"); region
// is a real AWS region code (e.g. "us-east-1").
func Fetch(client httpClient, serviceCode, region string) ([]pricing.PriceEntry, string, error) {
	idx, err := fetchRegionIndex(client, serviceCode)
	if err != nil {
		return nil, "", err
	}
	entry, ok := idx.Regions[region]
	if !ok {
		return nil, "", fmt.Errorf("pricingfetch: service %s has no price list for region %s", serviceCode, region)
	}

	offer, err := fetchOfferFile(client, entry.CurrentVersionURL)
	if err != nil {
		return nil, "", err
	}

	var out []pricing.PriceEntry
	for sku, product := range offer.Products {
		terms, ok := offer.Terms.OnDemand[sku]
		if !ok {
			continue // no on-demand pricing for this SKU — real, not an error
		}
		for _, term := range terms {
			for _, dim := range term.PriceDimensions {
				priceStr, ok := dim.PricePerUnit["USD"]
				if !ok {
					continue
				}
				price, err := strconv.ParseFloat(priceStr, 64)
				if err != nil {
					continue // a non-numeric USD price is real AWS data this fetcher can't use, not a crash
				}
				attrs := make(map[string]string, len(product.Attributes)+2)
				for k, v := range product.Attributes {
					attrs[k] = v
				}
				attrs["product_family"] = product.ProductFamily
				attrs["rate_description"] = dim.Description
				out = append(out, pricing.PriceEntry{
					Service:       serviceCode,
					Region:        region,
					SKUAttributes: attrs,
					Unit:          dim.Unit,
					Price:         price,
					Currency:      "USD",
				})
			}
		}
	}
	source := fmt.Sprintf("AWS Bulk Price List API, %s %s", serviceCode, offer.Version)
	return out, source, nil
}

// FetchSnapshot fetches every named service for one region and assembles one
// dated snapshot — the real, top-level entry point cmd/runnerd's own CLI calls.
func FetchSnapshot(client httpClient, snapshotID string, serviceCodes []string, region string) (pricing.Snapshot, error) {
	snap := pricing.Snapshot{
		ID:         snapshotID,
		FetchedAt:  time.Now().UTC(),
		Disclaimer: pricing.AWSDisclaimer,
	}
	var sources []string
	for _, svc := range serviceCodes {
		entries, source, err := Fetch(client, svc, region)
		if err != nil {
			return pricing.Snapshot{}, fmt.Errorf("pricingfetch: fetching %s: %w", svc, err)
		}
		snap.Entries = append(snap.Entries, entries...)
		sources = append(sources, source)
	}
	snap.Source = fmt.Sprintf("%v", sources)
	return snap, nil
}

func fetchRegionIndex(client httpClient, serviceCode string) (regionIndex, error) {
	url := fmt.Sprintf("%s/offers/v1.0/aws/%s/current/region_index.json", bulkAPIBase, serviceCode)
	body, err := get(client, url)
	if err != nil {
		return regionIndex{}, err
	}
	var idx regionIndex
	if err := json.Unmarshal(body, &idx); err != nil {
		return regionIndex{}, fmt.Errorf("pricingfetch: parse region index for %s: %w", serviceCode, err)
	}
	return idx, nil
}

func fetchOfferFile(client httpClient, path string) (offerFile, error) {
	body, err := get(client, bulkAPIBase+path)
	if err != nil {
		return offerFile{}, err
	}
	var offer offerFile
	if err := json.Unmarshal(body, &offer); err != nil {
		return offerFile{}, fmt.Errorf("pricingfetch: parse offer file %s: %w", path, err)
	}
	return offer, nil
}

func get(client httpClient, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("pricingfetch: GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pricingfetch: GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
