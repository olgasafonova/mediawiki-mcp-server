package registry

import "testing"

func TestDetectCountry(t *testing.T) {
	tests := []struct {
		orgNumber string
		expected  Country
	}{
		// Norway - 9 digits
		{"923609016", CountryNorway},
		{"923 609 016", CountryNorway},

		// Sweden - 10 digits
		{"5560360793", CountrySweden},
		{"556036-0793", CountrySweden},

		// Finland - 7 digits + hyphen + 1 digit
		{"0112038-9", CountryFinland},
		{"2331972-7", CountryFinland},

		// Ambiguous cases (8 digits without hyphen could be DK or FI)
		{"01120389", ""},
		{"25313763", ""},

		// Invalid
		{"12345", ""},
		{"abcdefghi", ""},
	}

	for _, tt := range tests {
		t.Run(tt.orgNumber, func(t *testing.T) {
			got := DetectCountry(tt.orgNumber)
			if got != tt.expected {
				t.Errorf("DetectCountry(%q) = %q, want %q", tt.orgNumber, got, tt.expected)
			}
		})
	}
}

func TestCleanOrgNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"923 609 016", "923609016"},
		{"556036-0793", "5560360793"},
		{"  123456789  ", "123456789"},
		{"12-34-56-78", "12345678"},
	}

	for _, tt := range tests {
		got := CleanOrgNumber(tt.input)
		if got != tt.expected {
			t.Errorf("CleanOrgNumber(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatOrgNumber(t *testing.T) {
	tests := []struct {
		orgNumber string
		country   Country
		expected  string
	}{
		{"923609016", CountryNorway, "923 609 016"},
		{"25313763", CountryDenmark, "25 31 37 63"},
		{"01120389", CountryFinland, "0112038-9"},
		{"5560360793", CountrySweden, "556036-0793"},
	}

	for _, tt := range tests {
		t.Run(string(tt.country), func(t *testing.T) {
			got := FormatOrgNumber(tt.orgNumber, tt.country)
			if got != tt.expected {
				t.Errorf("FormatOrgNumber(%q, %q) = %q, want %q", tt.orgNumber, tt.country, got, tt.expected)
			}
		})
	}
}

func TestValidateNorwayOrgNumber(t *testing.T) {
	tests := []struct {
		orgNumber string
		valid     bool
	}{
		// Valid Norwegian org numbers (verified against BRREG)
		{"923609016", true},  // Equinor
		{"914778271", true},  // DNB
		{"976389387", true},  // Telenor
		{"985615616", true},  // Norsk Hydro

		// Invalid check digits
		{"923609010", false},
		{"123456789", false},

		// Wrong length
		{"12345678", false},
		{"1234567890", false},
	}

	for _, tt := range tests {
		t.Run(tt.orgNumber, func(t *testing.T) {
			result := ValidateOrgNumber(tt.orgNumber, CountryNorway)
			if result.Valid != tt.valid {
				t.Errorf("ValidateOrgNumber(%q, NO) = %v, want %v", tt.orgNumber, result.Valid, tt.valid)
			}
		})
	}
}

func TestValidateDenmarkOrgNumber(t *testing.T) {
	tests := []struct {
		orgNumber string
		valid     bool
	}{
		// Valid Danish CVR numbers
		{"25313763", true}, // Novo Nordisk
		{"10150817", true}, // Maersk
		{"61126228", true}, // Carlsberg

		// Invalid
		{"25313760", false},
		{"1234567", false}, // Wrong length
	}

	for _, tt := range tests {
		t.Run(tt.orgNumber, func(t *testing.T) {
			result := ValidateOrgNumber(tt.orgNumber, CountryDenmark)
			if result.Valid != tt.valid {
				t.Errorf("ValidateOrgNumber(%q, DK) = %v, want %v", tt.orgNumber, result.Valid, tt.valid)
			}
		})
	}
}

func TestValidateFinlandOrgNumber(t *testing.T) {
	tests := []struct {
		orgNumber string
		valid     bool
	}{
		// Valid Finnish Y-tunnus (verified format)
		{"01120389", true}, // Nokia (0112038-9)
		{"23319727", true}, // Reaktor (2331972-7)

		// Invalid check digit
		{"01120380", false},

		// Wrong length
		{"123456789", false},
	}

	for _, tt := range tests {
		t.Run(tt.orgNumber, func(t *testing.T) {
			result := ValidateOrgNumber(CleanOrgNumber(tt.orgNumber), CountryFinland)
			if result.Valid != tt.valid {
				t.Errorf("ValidateOrgNumber(%q, FI) = %v, want %v", tt.orgNumber, result.Valid, tt.valid)
			}
		})
	}
}

func TestValidateSwedenOrgNumber(t *testing.T) {
	tests := []struct {
		orgNumber string
		valid     bool
	}{
		// Valid Swedish org numbers (Luhn algorithm)
		{"5560360793", true}, // Spotify
		{"5565475489", true}, // Volvo

		// Invalid check digit
		{"5560360790", false},

		// Wrong length
		{"123456789", false},
	}

	for _, tt := range tests {
		t.Run(tt.orgNumber, func(t *testing.T) {
			result := ValidateOrgNumber(tt.orgNumber, CountrySweden)
			if result.Valid != tt.valid {
				t.Errorf("ValidateOrgNumber(%q, SE) = %v, want %v", tt.orgNumber, result.Valid, tt.valid)
			}
		})
	}
}

func TestIsAllDigits(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"123456789", true},
		{"0000000000", true},
		{"", false},
		{"12345678a", false},
		{"12-34-56", false},
		{" 123", false},
	}

	for _, tt := range tests {
		got := isAllDigits(tt.input)
		if got != tt.expected {
			t.Errorf("isAllDigits(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestValidationResult(t *testing.T) {
	result := ValidateOrgNumber("923609016", CountryNorway)

	if !result.Valid {
		t.Error("expected valid result")
	}
	if result.Country != CountryNorway {
		t.Errorf("expected country NO, got %s", result.Country)
	}
	if result.FormattedNumber != "923 609 016" {
		t.Errorf("expected formatted number '923 609 016', got %s", result.FormattedNumber)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}
