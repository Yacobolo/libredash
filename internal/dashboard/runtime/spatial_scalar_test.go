package runtime

import (
	"math/big"
	"testing"
)

func TestNormalizeDatumValueCanonicalizesSafeDuckDBHugeIntegers(t *testing.T) {
	value := big.NewInt(805)
	got := normalizeDatumValue(value)
	if got != float64(805) {
		t.Fatalf("normalizeDatumValue(%T(%v)) = %T(%v), want float64(805)", value, value, got, got)
	}
}

func TestNormalizeDatumValueDoesNotRoundUnsafeDuckDBHugeIntegers(t *testing.T) {
	value := new(big.Int).SetUint64(1 << 53)
	if got := normalizeDatumValue(value); got != value {
		t.Fatalf("normalizeDatumValue(%v) = %T(%v), want original value so validation fails closed", value, got, got)
	}
}
