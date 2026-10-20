package models

import (
	"testing"
)

func TestCanonicalJSONSortsObjectKeys(t *testing.T) {
	got, err := CanonicalJSON(map[string]any{
		"b": 2,
		"a": map[string]any{"y": 1, "x": 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":{"x":0,"y":1},"b":2}`
	if string(got) != want {
		t.Fatalf("canonical JSON = %s, want %s", got, want)
	}
}

func TestQuantizationConfigHashHexNullAndAbsentMatch(t *testing.T) {
	nullHash, err := QuantizationConfigHashHex(nil)
	if err != nil {
		t.Fatal(err)
	}
	if nullHash != "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b" {
		t.Fatalf("unexpected canonical-null hash: %s", nullHash)
	}
}

func TestQuantizationConfigHashHexIgnoresKeyOrder(t *testing.T) {
	a, err := QuantizationConfigHashHex(map[string]any{
		"quant_method":           "bitsandbytes",
		"components_to_quantize": []any{"transformer"},
		"quant_kwargs":           map[string]any{"load_in_4bit": true, "bnb_4bit_quant_type": "nf4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := QuantizationConfigHashHex(map[string]any{
		"quant_kwargs":           map[string]any{"bnb_4bit_quant_type": "nf4", "load_in_4bit": true},
		"components_to_quantize": []any{"transformer"},
		"quant_method":           "bitsandbytes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("key order altered hash: %s vs %s", a, b)
	}
	nullHash, err := QuantizationConfigHashHex(nil)
	if err != nil {
		t.Fatal(err)
	}
	if a == nullHash {
		t.Fatal("4bit config hash must differ from null hash")
	}
}
