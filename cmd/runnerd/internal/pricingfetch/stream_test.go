package pricingfetch

import (
	"strings"
	"testing"
)

// A miniature offer file in the REAL shape (field names and nesting as in AmazonEC2/eu-west-1,
// 2026-10-02): products first, then terms {OnDemand, Reserved}. The Reserved block is
// deliberately INVALID JSON: a decoder that reads past OnDemand would fail on it.
const miniOffer = `{
 "formatVersion":"v1.0","disclaimer":"d","offerCode":"AmazonEC2","version":"20260925174521","publicationDate":"2026-09-25T17:45:21Z",
 "products":{
  "NAT1":{"sku":"NAT1","productFamily":"NAT Gateway","attributes":{"usagetype":"EU-NatGateway-Bytes","operation":"NatGateway"}},
  "INST":{"sku":"INST","productFamily":"Compute Instance","attributes":{"usagetype":"EU-BoxUsage:m5.large","instanceType":"m5.large"}},
  "NAT2":{"sku":"NAT2","productFamily":"NAT Gateway","attributes":{"usagetype":"EU-NatGateway-Hours","operation":"NatGateway"}}
 },
 "terms":{
  "OnDemand":{
   "NAT1":{"NAT1.JRTCKXETXF":{"priceDimensions":{"NAT1.JRTCKXETXF.6YS6EN2CT7":{"description":"per GB","beginRange":"0","endRange":"Inf","unit":"GB","pricePerUnit":{"USD":"0.0480000000"}}}}},
   "INST":{"INST.JRTCKXETXF":{"priceDimensions":{"INST.JRTCKXETXF.6YS6EN2CT7":{"description":"per hour","beginRange":"0","endRange":"Inf","unit":"Hrs","pricePerUnit":{"USD":"0.1000000000"}}}}},
   "NAT2":{"NAT2.JRTCKXETXF":{"priceDimensions":{"NAT2.JRTCKXETXF.6YS6EN2CT7":{"description":"per hour","beginRange":"0","endRange":"Inf","unit":"Hrs","pricePerUnit":{"USD":"0.0520000000"}}}}}
  },
  "Reserved": {{{ this is not json and must never be parsed
 }
}`

func TestDecodeOffer_FiltersToTheProductFamily_AndStopsBeforeReserved(t *testing.T) {
	off, err := decodeOffer(strings.NewReader(miniOffer), "NAT Gateway")
	if err != nil {
		t.Fatalf("decode must stop after OnDemand and never touch the Reserved block: %v", err)
	}
	if off.Version != "20260925174521" || off.ProductsSeen != 3 {
		t.Errorf("version=%q seen=%d, want the file's version and all 3 products counted", off.Version, off.ProductsSeen)
	}
	if len(off.Products) != 2 || off.Products["INST"].SKU != "" {
		t.Errorf("kept %v, want only the two NAT Gateway products", off.Products)
	}
	if len(off.OnDemand) != 2 || off.OnDemand["INST"] != nil {
		t.Errorf("on-demand terms kept for %v, want only the kept products", off.OnDemand)
	}
	if got := off.OnDemand["NAT1"]["NAT1.JRTCKXETXF"].PriceDimensions["NAT1.JRTCKXETXF.6YS6EN2CT7"].PricePerUnit["USD"]; got != "0.0480000000" {
		t.Errorf("NAT1 price = %q, want 0.0480000000", got)
	}
}

func TestDecodeOffer_NoFilter_KeepsEverything(t *testing.T) {
	off, err := decodeOffer(strings.NewReader(miniOffer), "")
	if err != nil || len(off.Products) != 3 || len(off.OnDemand) != 3 {
		t.Fatalf("an unfiltered service keeps every product: %d products, %d terms, err=%v", len(off.Products), len(off.OnDemand), err)
	}
}

// If a file ever put terms before products, filtering by product would silently drop terms:
// refuse instead.
func TestDecodeOffer_TermsBeforeProducts_IsRefused(t *testing.T) {
	_, err := decodeOffer(strings.NewReader(`{"terms":{"OnDemand":{}},"products":{}}`), "NAT Gateway")
	if err == nil || !strings.Contains(err.Error(), "precede") {
		t.Fatalf("got %v, want a loud refusal", err)
	}
}

func TestDecodeOffer_TruncatedFileIsAnError(t *testing.T) {
	if _, err := decodeOffer(strings.NewReader(`{"products":{"NAT1":{"sku":"NAT1","productFamily":"NAT Gateway"`), "NAT Gateway"); err == nil {
		t.Fatal("a truncated download must be an error, not a short snapshot")
	}
}

func ec2FakeClient() fakeHTTPClient {
	return fakeHTTPClient{byURLSuffix: map[string]string{
		"AmazonEC2/current/region_index.json":           "testdata/amazonec2_region_index.json",
		"AmazonEC2/20260925174521/eu-west-1/index.json": "testdata/amazonec2_euwest1_nat_offer.json",
	}}
}

// ADR-006 amendment: AmazonEC2 is fetched, but ONLY its NAT Gateway product family is stored.
// The fixture is a REAL extract (see testdata/README-amazonec2-capture.md): six NAT Gateway
// products plus two real instance products, which must be dropped at fetch time.
func TestFetch_AmazonEC2_StoresOnlyNATGatewayRows_RealData(t *testing.T) {
	entries, source, err := Fetch(ec2FakeClient(), "AmazonEC2", "eu-west-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 6 {
		t.Fatalf("got %d entries, want the 6 NAT Gateway rows (the 2 instance products must be dropped)", len(entries))
	}
	byUsage := map[string]float64{}
	units := map[string]string{}
	for _, e := range entries {
		if e.Service != "AmazonEC2" || e.Region != "eu-west-1" || e.Currency != "USD" || e.SKUAttributes["product_family"] != "NAT Gateway" {
			t.Errorf("unexpected entry %+v", e)
		}
		if e.SKUAttributes["productFamily"] == "Compute Instance" || strings.Contains(e.SKUAttributes["usagetype"], "BoxUsage") {
			t.Errorf("a non-NAT row leaked into the snapshot: %+v", e)
		}
		byUsage[e.SKUAttributes["usagetype"]] = e.Price
		units[e.SKUAttributes["usagetype"]] = e.Unit
	}
	// Real eu-west-1 rates and units (AmazonEC2 offer 20260925174521).
	if byUsage["EU-NatGateway-Hours"] != 0.048 || units["EU-NatGateway-Hours"] != "Hrs" {
		t.Errorf("NatGateway-Hours = %v %s, want 0.048 Hrs", byUsage["EU-NatGateway-Hours"], units["EU-NatGateway-Hours"])
	}
	if byUsage["EU-NatGateway-Bytes"] != 0.048 || units["EU-NatGateway-Bytes"] != "GB" {
		t.Errorf("NatGateway-Bytes = %v %s, want 0.048 GB", byUsage["EU-NatGateway-Bytes"], units["EU-NatGateway-Bytes"])
	}
	if !strings.Contains(source, "20260925174521") || !strings.Contains(source, `productFamily "NAT Gateway"`) || !strings.Contains(source, "6 of 8 products") {
		t.Errorf("the snapshot source must record the version and the filter: %q", source)
	}
}

// Rows come out in a deterministic order whatever the map iteration did (NFR-1).
func TestFetch_AmazonEC2_IsDeterministic(t *testing.T) {
	a, _, _ := Fetch(ec2FakeClient(), "AmazonEC2", "eu-west-1")
	for i := 0; i < 5; i++ {
		b, _, _ := Fetch(ec2FakeClient(), "AmazonEC2", "eu-west-1")
		for j := range a {
			if a[j].SKUAttributes["sku"] != b[j].SKUAttributes["sku"] {
				t.Fatalf("row %d differs between identical fetches", j)
			}
		}
	}
}
