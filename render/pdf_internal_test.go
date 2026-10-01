package render

import (
	"bytes"
	"testing"
)

func TestNormalizeDates_RewritesBothDatesToTheSameLengthConstant(t *testing.T) {
	in := []byte("<</Producer (Skia/PDF m153)\n/CreationDate (D:20261001195008+00'00')\n/ModDate (D:20261001195010+00'00')>>")
	out := normalizeDates(in)
	if len(out) != len(in) {
		t.Fatalf("length changed %d -> %d; every xref offset after it would be wrong", len(in), len(out))
	}
	if bytes.Contains(out, []byte("20261001")) {
		t.Errorf("a real date survived: %s", out)
	}
	want := "<</Producer (Skia/PDF m153)\n/CreationDate (" + fixedPDFDate + ")\n/ModDate (" + fixedPDFDate + ")>>"
	if string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// Negative control: the two renders in the real measurement differed ONLY in these dates, so
// two inputs differing only there must normalize to identical bytes — and two inputs that
// differ anywhere else must NOT (the normalizer hides nothing but the dates).
func TestNormalizeDates_HidesOnlyTheDates(t *testing.T) {
	a := []byte("/CreationDate (D:20261001195008+00'00') stream-A")
	b := []byte("/CreationDate (D:20261001195010+00'00') stream-A")
	c := []byte("/CreationDate (D:20261001195010+00'00') stream-B")
	if !bytes.Equal(normalizeDates(a), normalizeDates(b)) {
		t.Error("inputs differing only in the date must normalize identically")
	}
	if bytes.Equal(normalizeDates(b), normalizeDates(c)) {
		t.Error("the normalizer must not hide a real content difference")
	}
}

func TestNormalizeDates_HandlesOtherTimezoneOffsetsOfTheSameWidth(t *testing.T) {
	in := []byte("/ModDate (D:20261001195008-05'30')")
	out := normalizeDates(in)
	if len(out) != len(in) || bytes.Contains(out, []byte("1995")) {
		t.Errorf("a non-UTC offset must normalize too: %q", out)
	}
}
