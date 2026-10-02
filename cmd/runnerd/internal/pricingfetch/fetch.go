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
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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

// product, priceDimension and term are the real shapes inside one service/region's price list
// file — verified live against AWSELB/us-east-1 (2026-09-25) and AmazonEC2/eu-west-1
// (2026-10-02, ADR-006 amendment).
type product struct {
	SKU           string            `json:"sku"`
	ProductFamily string            `json:"productFamily"`
	Attributes    map[string]string `json:"attributes"`
}

type priceDimension struct {
	Description  string            `json:"description"`
	BeginRange   string            `json:"beginRange"`
	EndRange     string            `json:"endRange"`
	Unit         string            `json:"unit"`
	PricePerUnit map[string]string `json:"pricePerUnit"`
}

type term struct {
	PriceDimensions map[string]priceDimension `json:"priceDimensions"`
}

// productFamilyFilters names the services whose offer file is too large to keep whole, and the
// ONLY product families of it that are stored (ADR-006 amendment, 2026-10-02). AmazonEC2's file
// is ~441-481 MB per region; the one thing a current story needs from it is NAT Gateway pricing
// (hourly and per-GB data processing), so the offer is STREAMED and everything outside that
// product family is dropped as it is read — never held in memory, never stored. A service with
// no entry here is stored whole, as before.
var productFamilyFilters = map[string]string{
	"AmazonEC2": "NAT Gateway",
}

// offer is the filtered content of one price list file.
type offer struct {
	Version      string
	ProductsSeen int
	Products     map[string]product
	OnDemand     map[string]map[string]term
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

	family := productFamilyFilters[serviceCode]
	off, err := fetchOffer(client, entry.CurrentVersionURL, family)
	if err != nil {
		return nil, "", err
	}

	var out []pricing.PriceEntry
	for sku, product := range off.Products {
		terms, ok := off.OnDemand[sku]
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
				attrs["sku"] = sku
				attrs["product_family"] = product.ProductFamily
				attrs["rate_description"] = dim.Description
				attrs["begin_range"] = dim.BeginRange
				attrs["end_range"] = dim.EndRange
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
	// Products is a map: sort so identical input yields identical snapshots (NFR-1).
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.SKUAttributes["sku"] != b.SKUAttributes["sku"] {
			return a.SKUAttributes["sku"] < b.SKUAttributes["sku"]
		}
		return rangeStart(a) < rangeStart(b)
	})
	source := fmt.Sprintf("AWS Bulk Price List API, %s %s", serviceCode, off.Version)
	if family != "" {
		source += fmt.Sprintf(" (streamed; stored only productFamily %q: %d of %d products)", family, len(off.Products), off.ProductsSeen)
	}
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

// fetchOffer streams one price list file and keeps only what the snapshot needs: every product
// (family == "") or just those of one product family, with their on-demand terms. The body is
// never read into memory whole — AmazonEC2's is hundreds of MB — and reading stops once the
// on-demand terms are done (the much larger Reserved terms follow and are never needed).
func fetchOffer(client httpClient, path, family string) (offer, error) {
	resp, err := client.Get(bulkAPIBase + path)
	if err != nil {
		return offer{}, fmt.Errorf("pricingfetch: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return offer{}, fmt.Errorf("pricingfetch: GET %s: status %d", path, resp.StatusCode)
	}
	off, err := decodeOffer(resp.Body, family)
	if err != nil {
		return offer{}, fmt.Errorf("pricingfetch: parse offer file %s: %w", path, err)
	}
	return off, nil
}

// decodeOffer walks the offer file's JSON token by token. The file's own order is products,
// then terms; terms are only meaningful for products already seen, so a file with terms before
// products is rejected loudly rather than silently mis-filtered.
func decodeOffer(r io.Reader, family string) (offer, error) {
	dec := json.NewDecoder(r)
	off := offer{Products: map[string]product{}, OnDemand: map[string]map[string]term{}}
	if err := expectDelim(dec, '{'); err != nil {
		return off, err
	}
	sawProducts := false
	for dec.More() {
		key, err := readKey(dec)
		if err != nil {
			return off, err
		}
		switch key {
		case "version":
			if err := dec.Decode(&off.Version); err != nil {
				return off, err
			}
		case "products":
			sawProducts = true
			if err := expectDelim(dec, '{'); err != nil {
				return off, err
			}
			for dec.More() {
				sku, err := readKey(dec)
				if err != nil {
					return off, err
				}
				var p product
				if err := dec.Decode(&p); err != nil {
					return off, err
				}
				off.ProductsSeen++
				if family == "" || p.ProductFamily == family {
					off.Products[sku] = p
				}
			}
			if err := expectDelim(dec, '}'); err != nil {
				return off, err
			}
		case "terms":
			if !sawProducts {
				return off, errors.New("terms precede products — cannot filter by product; refusing to guess")
			}
			return off, decodeTerms(dec, &off)
		default:
			if err := skipValue(dec); err != nil {
				return off, err
			}
		}
	}
	return off, nil
}

// decodeTerms reads the "terms" object, keeping on-demand terms only for products kept, and
// returns as soon as OnDemand is done.
func decodeTerms(dec *json.Decoder, off *offer) error {
	if err := expectDelim(dec, '{'); err != nil {
		return err
	}
	for dec.More() {
		kind, err := readKey(dec)
		if err != nil {
			return err
		}
		if kind != "OnDemand" {
			if err := skipValue(dec); err != nil {
				return err
			}
			continue
		}
		if err := expectDelim(dec, '{'); err != nil {
			return err
		}
		for dec.More() {
			sku, err := readKey(dec)
			if err != nil {
				return err
			}
			if _, keep := off.Products[sku]; !keep {
				if err := skipValue(dec); err != nil {
					return err
				}
				continue
			}
			var terms map[string]term
			if err := dec.Decode(&terms); err != nil {
				return err
			}
			off.OnDemand[sku] = terms
		}
		return nil // everything needed is read; the (much larger) Reserved terms are never needed
	}
	return nil
}

func expectDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != want {
		return fmt.Errorf("expected %q, got %v", want, tok)
	}
	return nil
}

func readKey(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	k, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("expected an object key, got %v", tok)
	}
	return k, nil
}

// skipValue consumes one JSON value without buffering it.
func skipValue(dec *json.Decoder) error {
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := tok.(json.Delim); ok {
			if d == '{' || d == '[' {
				depth++
			} else {
				depth--
			}
		}
		if depth == 0 {
			return nil
		}
	}
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

func rangeStart(e pricing.PriceEntry) float64 {
	v, _ := strconv.ParseFloat(e.SKUAttributes["begin_range"], 64)
	return v
}
