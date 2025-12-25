package norway

import (
	"testing"

	"github.com/olgasafonova/nordic-registry-mcp-server/internal/registry"
)

func intPtr(i int) *int {
	return &i
}

func BenchmarkToCompany(b *testing.B) {
	enhet := &BRREGEnhet{
		Organisasjonsnummer: "923609016",
		Navn:                "EQUINOR ASA",
		Organisasjonsform: BRREGOrganisasjonsform{
			Kode:        "ASA",
			Beskrivelse: "Allmennaksjeselskap",
		},
		Registreringsdato: "1972-09-18",
		AntallAnsatte:     intPtr(21000),
		Forretningsadresse: &BRREGAdresse{
			Adresse:    []string{"Forusbeen 50"},
			Postnummer: "4035",
			Poststed:   "STAVANGER",
			Kommune:    "STAVANGER",
			Land:       "Norge",
		},
		Naeringskode1: &BRREGNaeringskode{
			Kode:        "06.100",
			Beskrivelse: "Utvinning av råolje",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = enhet.ToCompany()
	}
}

func BenchmarkCleanOrgNumber(b *testing.B) {
	inputs := []string{
		"923609016",
		"923 609 016",
		" 923-609-016 ",
		"NO923609016",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, input := range inputs {
			_ = registry.CleanOrgNumber(input)
		}
	}
}

func BenchmarkDetectCountry(b *testing.B) {
	numbers := []string{
		"923609016",   // Norway (9 digits)
		"0112038-9",   // Finland (Y-tunnus)
		"24256790",    // Denmark (8 digits)
		"5560360793",  // Sweden (10 digits)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, num := range numbers {
			_ = registry.DetectCountry(num)
		}
	}
}

func BenchmarkValidateOrgNumber(b *testing.B) {
	testCases := []struct {
		orgNum  string
		country registry.Country
	}{
		{"923609016", registry.CountryNorway},
		{"0112038-9", registry.CountryFinland},
		{"24256790", registry.CountryDenmark},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			_ = registry.ValidateOrgNumber(tc.orgNum, tc.country)
		}
	}
}
