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
		// GetCompanyByID uses query parameter, not path
		if r.URL.Query().Get("businessId") == "2331972-7" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Returns a search response format with companies array
			w.Write([]byte(`{
				"totalResults": 1,
				"companies": [{
					"businessId": {
						"value": "2331972-7",
						"registrationDate": "2010-03-22"
					},
					"names": [{
						"name": "Reaktor Innovations Oy",
						"type": "1",
						"registrationDate": "2010-03-22"
					}],
					"companyForms": [{
						"type": "OY",
						"descriptions": [{"languageCode": "3", "description": "Limited company"}],
						"registrationDate": "2010-03-22"
					}],
					"addresses": [{
						"type": 1,
						"street": "Mannerheimintie 2",
						"postCode": "00100",
						"postOffices": [{"city": "HELSINKI", "languageCode": "1"}]
					}],
					"mainBusinessLine": {
						"type": "62010",
						"descriptions": [{"languageCode": "3", "description": "Computer programming activities"}]
					},
					"tradeRegisterStatus": "1"
				}]
			}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"totalResults": 0, "companies": []}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
	}, nil)

	company, err := client.GetCompanyByID(context.Background(), "2331972-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if company.BusinessID.Value != "2331972-7" {
		t.Errorf("expected businessId 2331972-7, got %s", company.BusinessID.Value)
	}
	if company.GetCurrentName() != "Reaktor Innovations Oy" {
		t.Errorf("expected name 'Reaktor Innovations Oy', got %s", company.GetCurrentName())
	}
}

func TestToCompany(t *testing.T) {
	prh := PRHCompany{
		BusinessID: PRHBusinessID{
			Value:            "1234567-8",
			RegistrationDate: "2015-06-15",
		},
		Names: []PRHName{
			{
				Name: "Finnish Example Oy",
				Type: "1",
			},
		},
		CompanyForms: []PRHCompanyForm{
			{
				Type: "OY",
				Descriptions: []PRHDescription{
					{LanguageCode: "3", Description: "Limited company"},
				},
			},
		},
		Addresses: []PRHAddress{
			{
				Type:     1,
				Street:   "Mannerheimintie 1",
				PostCode: "00100",
				PostOffices: []PRHPostOffice{
					{City: "HELSINKI", LanguageCode: "1"},
				},
			},
		},
		MainBusinessLine: &PRHMainBusinessLine{
			Type: "62010",
			Descriptions: []PRHDescription{
				{LanguageCode: "3", Description: "Computer programming"},
			},
		},
		TradeRegisterStatus: "1",
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
	if company.BusinessAddress == nil {
		t.Fatal("expected business address")
	}
	if company.BusinessAddress.City != "HELSINKI" {
		t.Errorf("expected city HELSINKI, got %s", company.BusinessAddress.City)
	}
}

func TestSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") == "Nokia" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"totalResults": 2,
				"companies": [
					{
						"businessId": {"value": "0112038-9"},
						"names": [{"name": "Nokia Oyj", "type": "1"}],
						"companyForms": [{"type": "OYJ"}]
					},
					{
						"businessId": {"value": "0201237-8"},
						"names": [{"name": "Nokia Solutions and Networks Oy", "type": "1"}],
						"companyForms": [{"type": "OY"}]
					}
				]
			}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"totalResults":0,"companies":[]}`))
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
	if len(resp.Companies) != 2 {
		t.Fatalf("expected 2 companies, got %d", len(resp.Companies))
	}
	if resp.Companies[0].GetCurrentName() != "Nokia Oyj" {
		t.Errorf("expected Nokia Oyj, got %s", resp.Companies[0].GetCurrentName())
	}
}

func TestGetCurrentName(t *testing.T) {
	tests := []struct {
		name     string
		company  PRHCompany
		expected string
	}{
		{
			name: "active official name",
			company: PRHCompany{
				Names: []PRHName{
					{Name: "Current Name Oy", Type: "1", EndDate: ""},
				},
			},
			expected: "Current Name Oy",
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

func TestBankruptcyStatus(t *testing.T) {
	company := PRHCompany{
		BusinessID: PRHBusinessID{Value: "1111111-1"},
		Names: []PRHName{
			{Name: "Bankrupt Oy", Type: "1"},
		},
		CompanySituations: []PRHCompanySituation{
			{
				Type: "KONK", // Bankruptcy - currently mapped to Liquidated status
			},
		},
	}

	unified := company.ToCompany()

	// Note: KONK is currently mapped to Liquidated (via IsInLiquidation check)
	// This could be improved to distinguish bankruptcy from voluntary liquidation
	if unified.Status != registry.StatusLiquidated {
		t.Errorf("expected status liquidated for KONK, got %s", unified.Status)
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
