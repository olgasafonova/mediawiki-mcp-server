// Package denmark provides a client for the Danish CVR (Central Business Register) API.
package denmark

import (
	"fmt"
	"time"
)

// CVRCompany represents a company from the cvrapi.dk API.
type CVRCompany struct {
	VAT              int              `json:"vat"`
	Name             string           `json:"name"`
	Address          string           `json:"address"`
	Zipcode          string           `json:"zipcode"`
	City             string           `json:"city"`
	CityName         *string          `json:"cityname"`
	Protected        bool             `json:"protected"`
	Phone            string           `json:"phone"`
	Email            string           `json:"email"`
	Fax              string           `json:"fax"`
	StartDate        string           `json:"startdate"`
	EndDate          *string          `json:"enddate"`
	Employees        *int             `json:"employees"`
	AddressCO        string           `json:"addressco"`
	IndustryCode     int              `json:"industrycode"`
	IndustryDesc     string           `json:"industrydesc"`
	CompanyCode      int              `json:"companycode"`
	CompanyDesc      string           `json:"companydesc"`
	CreditStartDate  *string          `json:"creditstartdate"`
	CreditBankrupt   bool             `json:"creditbankrupt"`
	CreditStatus     *string          `json:"creditstatus"`
	Owners           []CVROwner       `json:"owners"`
	ProductionUnits  []CVRProdUnit    `json:"productionunits"`
	TNumber          *int             `json:"t,omitempty"`
	Version          int              `json:"version,omitempty"`
	Slug             string           `json:"slug,omitempty"`
	Life             *CVRLife         `json:"life,omitempty"`
}

// CVROwner represents a company owner.
type CVROwner struct {
	Name string `json:"name"`
}

// CVRProdUnit represents a production unit (branch).
type CVRProdUnit struct {
	PNO          int         `json:"pno"`
	Main         bool        `json:"main"`
	Name         string      `json:"name"`
	Address      string      `json:"address"`
	Zipcode      string      `json:"zipcode"`
	City         string      `json:"city"`
	CityName     *string     `json:"cityname"`
	Protected    bool        `json:"protected"`
	Phone        string      `json:"phone"`
	Email        string      `json:"email"`
	Fax          string      `json:"fax"`
	StartDate    string      `json:"startdate"`
	EndDate      *string     `json:"enddate"`
	Employees    interface{} `json:"employees"` // Can be int, string, or null
	AddressCO    string      `json:"addressco"`
	IndustryCode int         `json:"industrycode"`
	IndustryDesc string      `json:"industrydesc"`
}

// CVRLife represents company lifecycle information.
type CVRLife struct {
	Start    string  `json:"start,omitempty"`
	End      *string `json:"end,omitempty"`
	Name     string  `json:"name,omitempty"`
	AddrCode string  `json:"addrcode,omitempty"`
}

// CVRSearchResponse represents a search response (list of companies).
type CVRSearchResponse struct {
	Companies []CVRCompany `json:"companies"`
}

// CVRError represents an error response from the API.
type CVRError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	T       int    `json:"t"`
}

// parseDate parses a CVR date string (DD/MM - YYYY format).
func parseDate(dateStr string) *time.Time {
	if dateStr == "" {
		return nil
	}

	// CVR uses format like "28/11 - 1931"
	t, err := time.Parse("02/01 - 2006", dateStr)
	if err != nil {
		// Try alternative format "YYYY-MM-DD"
		t, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil
		}
	}
	return &t
}

// FormatCVRNumber formats a CVR number with standard formatting.
func FormatCVRNumber(cvr int) string {
	return fmt.Sprintf("%08d", cvr)
}

// GetCVRString returns the CVR number as a formatted string.
func (c *CVRCompany) GetCVRString() string {
	return FormatCVRNumber(c.VAT)
}

// IsActive checks if the company is currently active.
func (c *CVRCompany) IsActive() bool {
	return c.EndDate == nil && !c.CreditBankrupt
}

// IsBankrupt checks if the company is bankrupt.
func (c *CVRCompany) IsBankrupt() bool {
	return c.CreditBankrupt
}

// GetMainProductionUnit returns the main production unit if any.
func (c *CVRCompany) GetMainProductionUnit() *CVRProdUnit {
	for i := range c.ProductionUnits {
		if c.ProductionUnits[i].Main {
			return &c.ProductionUnits[i]
		}
	}
	return nil
}
