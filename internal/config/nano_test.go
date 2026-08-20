package config

import "testing"

func TestParseNanoLocaleIndependent(t *testing.T) {
	t.Parallel()

	tests := map[string]Nano{
		"275.48":        275_480_000_000,
		"1 028,40":      1_028_400_000_000,
		"1\u00a0028,40": 1_028_400_000_000,
		"0.000000001":   1,
		"-0.500000000":  -500_000_000,
		"-1.25":         -1_250_000_000,
	}
	for input, expected := range tests {
		input, expected := input, expected
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			actual, err := ParseNano(input)
			if err != nil {
				t.Fatalf("parse nano: %v", err)
			}
			if actual != expected {
				t.Fatalf("value = %d, want %d", actual, expected)
			}
		})
	}
}

func TestNanoString(t *testing.T) {
	t.Parallel()

	tests := map[Nano]string{
		0:              "0.000000000",
		1:              "0.000000001",
		-500_000_000:   "-0.500000000",
		12_345_678_901: "12.345678901",
	}
	for value, expected := range tests {
		if actual := value.String(); actual != expected {
			t.Errorf("Nano(%d).String() = %q, want %q", value, actual, expected)
		}
	}
}

func TestParseNanoRejectsAmbiguousOrImpreciseValues(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", "1,000.00", "1.0000000001", "NaN", "1,2,3", "1.", "1,"} {
		if _, err := ParseNano(input); err == nil {
			t.Fatalf("ParseNano(%q) succeeded, want error", input)
		}
	}
}
