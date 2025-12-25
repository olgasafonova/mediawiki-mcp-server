package registry

import (
	"regexp"
	"strconv"
	"strings"
)

// OrgNumberFormat describes the format of an organization number.
type OrgNumberFormat struct {
	Country   Country
	Length    int
	Pattern   string
	Algorithm string // MOD11, Luhn, etc.
}

var orgNumberFormats = []OrgNumberFormat{
	{Country: CountryNorway, Length: 9, Pattern: `^\d{9}$`, Algorithm: "MOD11"},
	{Country: CountryDenmark, Length: 8, Pattern: `^\d{8}$`, Algorithm: "MOD11"},
	{Country: CountryFinland, Length: 8, Pattern: `^\d{7}-\d$`, Algorithm: "MOD11"},
	{Country: CountrySweden, Length: 10, Pattern: `^\d{10}$`, Algorithm: "Luhn"},
}

// DetectCountry attempts to determine the country from an org number format.
// Returns empty Country if ambiguous or unrecognized.
func DetectCountry(orgNumber string) Country {
	cleaned := CleanOrgNumber(orgNumber)

	// Finland: 7 digits + hyphen + 1 digit (1234567-8)
	// Check original format first (before cleaning removes hyphen)
	if matched, _ := regexp.MatchString(`^\d{7}-\d$`, strings.TrimSpace(orgNumber)); matched {
		return CountryFinland
	}

	// Norway: 9 digits
	if len(cleaned) == 9 && isAllDigits(cleaned) {
		return CountryNorway
	}

	// Sweden: 10 digits
	if len(cleaned) == 10 && isAllDigits(cleaned) {
		return CountrySweden
	}

	// 8 pure digits (no hyphen) = Denmark
	// Finland always uses hyphen format (1234567-8), checked above
	if len(cleaned) == 8 && isAllDigits(cleaned) {
		return CountryDenmark
	}

	return ""
}

// CleanOrgNumber removes formatting characters from an org number.
func CleanOrgNumber(orgNumber string) string {
	// Remove spaces, hyphens (except Finnish format), and other separators
	cleaned := strings.ReplaceAll(orgNumber, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.TrimSpace(cleaned)
	return cleaned
}

// FormatOrgNumber formats an org number according to country conventions.
func FormatOrgNumber(orgNumber string, country Country) string {
	cleaned := CleanOrgNumber(orgNumber)

	switch country {
	case CountryNorway:
		// 123 456 789
		if len(cleaned) == 9 {
			return cleaned[:3] + " " + cleaned[3:6] + " " + cleaned[6:]
		}
	case CountryDenmark:
		// 12 34 56 78
		if len(cleaned) == 8 {
			return cleaned[:2] + " " + cleaned[2:4] + " " + cleaned[4:6] + " " + cleaned[6:]
		}
	case CountryFinland:
		// 1234567-8
		if len(cleaned) == 8 {
			return cleaned[:7] + "-" + cleaned[7:]
		}
	case CountrySweden:
		// 123456-7890
		if len(cleaned) == 10 {
			return cleaned[:6] + "-" + cleaned[6:]
		}
	}

	return orgNumber
}

// ValidateOrgNumber validates an organization number for a specific country.
func ValidateOrgNumber(orgNumber string, country Country) ValidationResult {
	cleaned := CleanOrgNumber(orgNumber)

	result := ValidationResult{
		OrgNumber:       orgNumber,
		FormattedNumber: FormatOrgNumber(orgNumber, country),
		Country:         country,
	}

	switch country {
	case CountryNorway:
		result.Valid, result.CheckDigitValid = validateNorwayOrgNumber(cleaned)
	case CountryDenmark:
		result.Valid, result.CheckDigitValid = validateDenmarkOrgNumber(cleaned)
	case CountryFinland:
		result.Valid, result.CheckDigitValid = validateFinlandOrgNumber(cleaned)
	case CountrySweden:
		result.Valid, result.CheckDigitValid = validateSwedenOrgNumber(cleaned)
	default:
		result.Message = "Unknown country"
		return result
	}

	if result.Valid {
		result.Message = "Valid " + country.Name() + " organization number"
	} else if !result.CheckDigitValid {
		result.Message = "Invalid check digit"
	} else {
		result.Message = "Invalid format for " + country.Name()
	}

	return result
}

// validateNorwayOrgNumber validates a Norwegian org number using MOD11.
// Norwegian org numbers are 9 digits with the last digit being a MOD11 check digit.
func validateNorwayOrgNumber(cleaned string) (valid, checkDigitValid bool) {
	if len(cleaned) != 9 || !isAllDigits(cleaned) {
		return false, false
	}

	// MOD11 weights for positions 1-8
	weights := []int{3, 2, 7, 6, 5, 4, 3, 2}
	sum := 0

	for i := 0; i < 8; i++ {
		digit, _ := strconv.Atoi(string(cleaned[i]))
		sum += digit * weights[i]
	}

	remainder := sum % 11
	var expectedCheckDigit int

	if remainder == 0 {
		expectedCheckDigit = 0
	} else {
		expectedCheckDigit = 11 - remainder
	}

	// If result is 10, the number is invalid
	if expectedCheckDigit == 10 {
		return false, false
	}

	actualCheckDigit, _ := strconv.Atoi(string(cleaned[8]))
	checkDigitValid = expectedCheckDigit == actualCheckDigit

	return checkDigitValid, checkDigitValid
}

// validateDenmarkOrgNumber validates a Danish CVR number using MOD11.
// Danish CVR numbers are 8 digits with the last digit being a MOD11 check digit.
func validateDenmarkOrgNumber(cleaned string) (valid, checkDigitValid bool) {
	if len(cleaned) != 8 || !isAllDigits(cleaned) {
		return false, false
	}

	// MOD11 weights for positions 1-7
	weights := []int{2, 7, 6, 5, 4, 3, 2}
	sum := 0

	for i := 0; i < 7; i++ {
		digit, _ := strconv.Atoi(string(cleaned[i]))
		sum += digit * weights[i]
	}

	remainder := sum % 11
	var expectedCheckDigit int

	if remainder == 0 {
		expectedCheckDigit = 0
	} else {
		expectedCheckDigit = 11 - remainder
	}

	// If result is 10, the number is invalid
	if expectedCheckDigit == 10 {
		return false, false
	}

	actualCheckDigit, _ := strconv.Atoi(string(cleaned[7]))
	checkDigitValid = expectedCheckDigit == actualCheckDigit

	return checkDigitValid, checkDigitValid
}

// validateFinlandOrgNumber validates a Finnish Y-tunnus using MOD11.
// Finnish business IDs are 7 digits + check digit (format: 1234567-8).
func validateFinlandOrgNumber(cleaned string) (valid, checkDigitValid bool) {
	if len(cleaned) != 8 || !isAllDigits(cleaned) {
		return false, false
	}

	// MOD11 weights for positions 1-7
	weights := []int{7, 9, 10, 5, 8, 4, 2}
	sum := 0

	for i := 0; i < 7; i++ {
		digit, _ := strconv.Atoi(string(cleaned[i]))
		sum += digit * weights[i]
	}

	remainder := sum % 11
	var expectedCheckDigit int

	if remainder == 0 {
		expectedCheckDigit = 0
	} else if remainder == 1 {
		// Invalid - no valid check digit exists
		return false, false
	} else {
		expectedCheckDigit = 11 - remainder
	}

	actualCheckDigit, _ := strconv.Atoi(string(cleaned[7]))
	checkDigitValid = expectedCheckDigit == actualCheckDigit

	return checkDigitValid, checkDigitValid
}

// validateSwedenOrgNumber validates a Swedish org number using Luhn algorithm.
// Swedish org numbers are 10 digits (YYMMDD-XXXX or 16XXXXXX-XXXX format).
func validateSwedenOrgNumber(cleaned string) (valid, checkDigitValid bool) {
	if len(cleaned) != 10 || !isAllDigits(cleaned) {
		return false, false
	}

	// Luhn algorithm
	sum := 0
	for i := 0; i < 10; i++ {
		digit, _ := strconv.Atoi(string(cleaned[i]))

		// Double every other digit starting from the first
		if i%2 == 0 {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
	}

	checkDigitValid = sum%10 == 0
	return checkDigitValid, checkDigitValid
}

// isAllDigits checks if a string contains only digits.
func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
