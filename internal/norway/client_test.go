package norway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/olgasafonova/nordic-registry-mcp-server/internal/registry"
)

func TestGetEnhet(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/enhetsregisteret/api/enheter/123456789" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"organisasjonsnummer": "123456789",
				"navn": "Test Company AS",
				"organisasjonsform": {
					"kode": "AS",
					"beskrivelse": "Aksjeselskap"
				},
				"forretningsadresse": {
					"adresse": ["Testgata 1"],
					"postnummer": "0123",
					"poststed": "OSLO",
					"kommune": "OSLO",
					"land": "Norge"
				},
				"antallAnsatte": 10,
				"registrertIForetaksregisteret": true
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
	}, nil)

	enhet, err := client.GetEnhet(context.Background(), "123456789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if enhet.Organisasjonsnummer != "123456789" {
		t.Errorf("expected org number 123456789, got %s", enhet.Organisasjonsnummer)
	}
	if enhet.Navn != "Test Company AS" {
		t.Errorf("expected name 'Test Company AS', got %s", enhet.Navn)
	}
	if enhet.Organisasjonsform.Kode != "AS" {
		t.Errorf("expected legal form AS, got %s", enhet.Organisasjonsform.Kode)
	}
}

func TestToCompany(t *testing.T) {
	employees := 42
	enhet := BRREGEnhet{
		Organisasjonsnummer: "987654321",
		Navn:                "Nordic Example AS",
		Organisasjonsform: BRREGOrganisasjonsform{
			Kode:        "AS",
			Beskrivelse: "Aksjeselskap",
		},
		Forretningsadresse: &BRREGAdresse{
			Adresse:    []string{"Karl Johans gate 1", "2. etasje"},
			Postnummer: "0154",
			Poststed:   "OSLO",
			Kommune:    "OSLO",
			Land:       "Norge",
		},
		Naeringskode1: &BRREGNaeringskode{
			Kode:        "62.010",
			Beskrivelse: "Programmeringstjenester",
		},
		AntallAnsatte:            &employees,
		Registreringsdato: "2020-01-15",
	}

	company := enhet.ToCompany()

	if company.Country != registry.CountryNorway {
		t.Errorf("expected country NO, got %s", company.Country)
	}
	if company.Name != "Nordic Example AS" {
		t.Errorf("expected name 'Nordic Example AS', got %s", company.Name)
	}
	if company.Status != registry.StatusActive {
		t.Errorf("expected status active, got %s", company.Status)
	}
	if company.LegalFormCode != "AS" {
		t.Errorf("expected legal form AS, got %s", company.LegalFormCode)
	}
	if company.Employees == nil || *company.Employees != 42 {
		t.Errorf("expected 42 employees, got %v", company.Employees)
	}
	if company.BusinessAddress == nil {
		t.Fatal("expected business address")
	}
	if company.BusinessAddress.City != "OSLO" {
		t.Errorf("expected city OSLO, got %s", company.BusinessAddress.City)
	}
	if len(company.IndustryCodes) != 1 {
		t.Fatalf("expected 1 industry code, got %d", len(company.IndustryCodes))
	}
	if company.IndustryCodes[0].Code != "62.010" {
		t.Errorf("expected industry code 62.010, got %s", company.IndustryCodes[0].Code)
	}
}

func TestSearchEnheter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("navn") == "Equinor" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"_embedded": {
					"enheter": [
						{
							"organisasjonsnummer": "923609016",
							"navn": "EQUINOR ASA",
							"organisasjonsform": {"kode": "ASA", "beskrivelse": "Allmennaksjeselskap"}
						}
					]
				},
				"page": {
					"size": 20,
					"totalElements": 1,
					"totalPages": 1,
					"number": 0
				}
			}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"_embedded":{"enheter":[]},"page":{"size":20,"totalElements":0,"totalPages":0,"number":0}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
	}, nil)

	resp, err := client.SearchEnheter(context.Background(), SearchParams{
		Navn: "Equinor",
		Size: 20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Page.TotalElements != 1 {
		t.Errorf("expected 1 result, got %d", resp.Page.TotalElements)
	}
	if len(resp.Embedded.Enheter) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(resp.Embedded.Enheter))
	}
	if resp.Embedded.Enheter[0].Navn != "EQUINOR ASA" {
		t.Errorf("expected EQUINOR ASA, got %s", resp.Embedded.Enheter[0].Navn)
	}
}

func TestGetEnhetNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
	}, nil)

	_, err := client.GetEnhet(context.Background(), "000000000")
	if err == nil {
		t.Fatal("expected error for not found entity")
	}
}

func TestBankruptStatus(t *testing.T) {
	enhet := BRREGEnhet{
		Organisasjonsnummer: "111111111",
		Navn:                "Bankrupt Company AS",
		Organisasjonsform:   BRREGOrganisasjonsform{Kode: "AS"},
		Konkurs:             true,
	}

	company := enhet.ToCompany()

	if company.Status != registry.StatusBankrupt {
		t.Errorf("expected status bankrupt, got %s", company.Status)
	}
}
