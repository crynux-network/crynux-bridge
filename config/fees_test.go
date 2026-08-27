package config

import (
	"math/big"
	"testing"
)

func TestCNXToWeiSupportsExactDecimalFees(t *testing.T) {
	feeWei, err := CNXToWei("0.0001")
	if err != nil {
		t.Fatal(err)
	}
	if feeWei != "100000000000000" {
		t.Fatalf("expected exact Wei, got %s", feeWei)
	}
}

func TestCNXToWeiRejectsInvalidPrecision(t *testing.T) {
	for _, value := range []string{"-0.0001", "1.0000000001", "1e-5", ""} {
		if _, err := CNXToWei(value); err == nil {
			t.Fatalf("expected error for %q", value)
		}
	}
}

func TestValidateWeiUint256Boundary(t *testing.T) {
	max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	if got, err := ValidateWei(max.String()); err != nil || got != max.String() {
		t.Fatalf("max uint256 rejected: %q, %v", got, err)
	}
	over := new(big.Int).Add(max, big.NewInt(1))
	if _, err := ValidateWei(over.String()); err == nil {
		t.Fatal("uint256 overflow accepted")
	}
}

func TestMultiplyWeiSupportsLargeFees(t *testing.T) {
	got, err := MultiplyWei("20000000000000000000", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != "40000000000000000000" {
		t.Fatalf("got %s", got)
	}
}
