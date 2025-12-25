package finland

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/olgasafonova/nordic-registry-mcp-server/internal/registry"
)

func TestGetCompanyByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/2331972-7" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"type": "fi.prh.opendata.bis",
				"version": "1",
				"totalResults": 1,
				"resultsFrom": 0,
				"results": [{
					"businessId": "2331972-7",
					"name": "Reaktor Innovations Oy",
					"registrationDate": "2010-03-22",
					"companyForm": "OY",
					"addresses": [{
						"street": "Mannerheimintie 2",
						"postCode": "00100",
						"city": "HELSINKI",
						"country": "FI",
						"type": 1
					}],
					"businessLines": [{
						"code": "62010",
						"name": "Computer programming activities",
						"order": 1
					}]
				}]
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
	}, nil)

	company, err := client.GetCompanyByID(context.Background(), "2331972-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if company.BusinessID != "2331972-7" {
		t.Errorf("expected businessId 2331972-7, got %s", company.BusinessID)
	}
	if company.Name != "Reaktor Innovations Oy" {
		t.Errorf("expected name 'Reaktor Innovations Oy', got %s", company.Name)
	}
}

func TestToCompany(t *testing.T) {
	prh := PRHCompany{
		BusinessID:       "1234567-8",
		Name:             "Finnish Example Oy",
		RegistrationDate: "2015-06-15",
		CompanyForm:      "OY",
		Addresses: []PRHAddress{
			{
				Street:   "Mannerheimintie 1",
				PostCode: "00100",
				City:     "HELSINKI",
				Country:  "FI",
				Type:     1,
			},
		},
		BusinessLines: []PRHBusinessLine{
			{
				Code: "62010",
				Name: "Computer programming",
			},
		},
	}

	company := prh.ToCompany()

	if company.Country != registry.CountryFinland {
		t.Errorf("expected country FI, got %s", company.Country)
	}
	if company.Name != "Finnish Example Oy" {
		t.Errorf("expected name 'Finnish Example Oy', got %s", company.Name)
	}
	if company.Status != registry.StatusActive {
		t.Errorf("expected status active, got %s", company.Status)
	}
	if company.LegalFormName != "OY" {
		t.Errorf("expected legal form OY, got %s", company.LegalFormName)
	}
	if company.BusinessAddress == nil {
		t.Fatal("expected business address")
	}
	if company.BusinessAddress.City != "HELSINKI" {
		t.Errorf("expected city HELSINKI, got %s", company.BusinessAddress.City)
	}
	if len(company.IndustryCodes) != 1 {
		t.Fatalf("expected 1 industry code, got %d", len(company.IndustryCodes))
	}
}

func TestSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") == "Nokia" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"type": "fi.prh.opendata.bis",
				"version": "1",
				"totalResults": 2,
				"resultsFrom": 0,
				"results": [
					{
						"businessId": "0112038-9",
						"name": "Nokia Oyj",
						"registrationDate": "1967-01-01",
						"companyForm": "OYJ"
					},
					{
						"businessId": "0201237-8",
						"name": "Nokia Solutions and Networks Oy",
						"registrationDate": "2007-04-01",
						"companyForm": "OY"
					}
				]
			}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"fi.prh.opendata.bis","version":"1","totalResults":0,"resultsFrom":0,"results":[]}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
	}, nil)

	resp, err := client.Search(context.Background(), SearchParams{
		Name: "Nokia",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.TotalResults != 2 {
		t.Errorf("expected 2 results, got %d", resp.TotalResults)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 companies, got %d", len(resp.Results))
	}
	if resp.Results[0].Name != "Nokia Oyj" {
		t.Errorf("expected Nokia Oyj, got %s", resp.Results[0].Name)
	}
}

func TestGetCurrentName(t *testing.T) {
	tests := []struct {
		name     string
		company  PRHCompany
		expected string
	}{
		{
			name: "top-level name",
			company: PRHCompany{
				Name: "Direct Name Oy",
			},
			expected: "Direct Name Oy",
		},
		{
			name: "active name in array",
			company: PRHCompany{
				Names: []PRHName{
					{Name: "Old Name Oy", EndDate: "2020-01-01"},
					{Name: "Current Name Oy", EndDate: ""},
				},
			},
			expected: "Current Name Oy",
		},
		{
			name: "fallback to first name",
			company: PRHCompany{
				Names: []PRHName{
					{Name: "Only Name Oy", EndDate: "2020-01-01"},
				},
			},
			expected: "Only Name Oy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.company.GetCurrentName()
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestLiquidationStatus(t *testing.T) {
	company := PRHCompany{
		BusinessID: "1111111-1",
		Name:       "Liquidated Oy",
		Liquidations: []PRHLiquidation{
			{
				Name:    "Liquidation",
				EndDate: "", // Active liquidation
			},
		},
	}

	unified := company.ToCompany()

	if unified.Status != registry.StatusLiquidated {
		t.Errorf("expected status liquidated, got %s", unified.Status)
	}
}

func TestCleanBusinessID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1234567-8", "12345678"},
		{"1234567 - 8", "12345678"},
		{"  1234567-8  ", "12345678"},
		{"12345678", "12345678"},
	}

	for _, tt := range tests {
		got := cleanBusinessID(tt.input)
		if got != tt.expected {
			t.Errorf("cleanBusinessID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
