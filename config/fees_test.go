package config

import "testing"

func TestCNXToGWeiSupportsDecimalFees(t *testing.T) {
	feeGWei, err := CNXToGWei(0.0001)
	if err != nil {
		t.Fatal(err)
	}

	if feeGWei != 100000 {
		t.Fatalf("expected 100000 GWei, got %d", feeGWei)
	}
}

func TestCNXToGWeiRejectsNegativeFees(t *testing.T) {
	if _, err := CNXToGWei(-0.0001); err == nil {
		t.Fatal("expected error for negative fee")
	}
}
