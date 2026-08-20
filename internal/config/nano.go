package config

import (
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

const NanoPerUnit int64 = 1_000_000_000

// Nano is a signed fixed-point decimal with nine fractional digits.
type Nano int64

func ParseNano(value string) (Nano, error) {
	normalized := strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) {
			return -1
		}
		return character
	}, strings.TrimSpace(value))
	if normalized == "" {
		return 0, fmt.Errorf("decimal is empty")
	}
	if strings.Contains(normalized, ".") && strings.Contains(normalized, ",") {
		return 0, fmt.Errorf("decimal has ambiguous separators")
	}
	if strings.Count(normalized, ".")+strings.Count(normalized, ",") > 1 {
		return 0, fmt.Errorf("decimal has multiple separators")
	}

	sign := int64(1)
	if strings.HasPrefix(normalized, "-") {
		sign = -1
		normalized = strings.TrimPrefix(normalized, "-")
	} else {
		normalized = strings.TrimPrefix(normalized, "+")
	}
	if normalized == "" {
		return 0, fmt.Errorf("decimal has no digits")
	}

	integerPart := normalized
	fractionalPart := ""
	if separator := strings.IndexAny(normalized, ".,"); separator >= 0 {
		integerPart = normalized[:separator]
		fractionalPart = normalized[separator+1:]
		if integerPart == "" || fractionalPart == "" {
			return 0, fmt.Errorf("decimal has invalid format")
		}
	}
	if !onlyDigits(integerPart) || !onlyDigits(fractionalPart) {
		return 0, fmt.Errorf("decimal contains non-digit characters")
	}
	if len(fractionalPart) > 9 {
		return 0, fmt.Errorf("decimal has more than nine fractional digits")
	}
	fractionalPart += strings.Repeat("0", 9-len(fractionalPart))

	combined := new(big.Int)
	if _, ok := combined.SetString(integerPart+fractionalPart, 10); !ok {
		return 0, fmt.Errorf("parse decimal")
	}
	if sign < 0 {
		combined.Neg(combined)
	}
	if !combined.IsInt64() {
		return 0, fmt.Errorf("decimal overflows nano representation")
	}
	return Nano(combined.Int64()), nil
}

func (value Nano) String() string {
	absolute := big.NewInt(int64(value))
	sign := ""
	if absolute.Sign() < 0 {
		sign = "-"
		absolute.Abs(absolute)
	}
	integer := new(big.Int)
	fraction := new(big.Int)
	integer.QuoRem(absolute, big.NewInt(NanoPerUnit), fraction)
	fractionDigits := fraction.String()
	return sign + integer.String() + "." + strings.Repeat("0", 9-len(fractionDigits)) + fractionDigits
}

func onlyDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
