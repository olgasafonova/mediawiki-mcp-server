// Package finland provides a client for the Finnish PRH (Patent and Registration Office) API.
package finland

import "time"

// PRHCompany represents a company from the PRH YTJ API v3.
type PRHCompany struct {
	BusinessID       PRHBusinessID       `json:"businessId"`
	EuID             *PRHEuID            `json:"euId,omitempty"`
	Names            []PRHName           `json:"names,omitempty"`
	MainBusinessLine *PRHMainBusinessLine `json:"mainBusinessLine,omitempty"`
	Website          *PRHWebsite         `json:"website,omitempty"`
	CompanyForms     []PRHCompanyForm    `json:"companyForms,omitempty"`
	CompanySituations []PRHCompanySituation `json:"companySituations,omitempty"`
	RegisteredEntries []PRHRegisteredEntry `json:"registeredEntries,omitempty"`
	Addresses        []PRHAddress        `json:"addresses,omitempty"`
	TradeRegisterStatus string           `json:"tradeRegisterStatus,omitempty"`
	Status           string              `json:"status,omitempty"`
	RegistrationDate string              `json:"registrationDate,omitempty"`
	EndDate          string              `json:"endDate,omitempty"`
	LastModified     string              `json:"lastModified,omitempty"`
}

// PRHBusinessID represents the Y-tunnus (business ID).
type PRHBusinessID struct {
	Value            string `json:"value"`
	RegistrationDate string `json:"registrationDate,omitempty"`
	Source           string `json:"source,omitempty"`
}

// PRHEuID represents the EU identifier.
type PRHEuID struct {
	Value  string `json:"value"`
	Source string `json:"source,omitempty"`
}

// PRHName represents a company name with validity period.
type PRHName struct {
	Name             string `json:"name"`
	Type             string `json:"type,omitempty"` // 1=official, 2=parallel, 3=auxiliary
	RegistrationDate string `json:"registrationDate,omitempty"`
	EndDate          string `json:"endDate,omitempty"`
	Version          int    `json:"version,omitempty"`
	Source           string `json:"source,omitempty"`
}

// PRHMainBusinessLine represents the main business activity.
type PRHMainBusinessLine struct {
	Type             string           `json:"type,omitempty"`
	Descriptions     []PRHDescription `json:"descriptions,omitempty"`
	TypeCodeSet      string           `json:"typeCodeSet,omitempty"`
	RegistrationDate string           `json:"registrationDate,omitempty"`
	Source           string           `json:"source,omitempty"`
}

// PRHDescription represents a localized description.
type PRHDescription struct {
	LanguageCode string `json:"languageCode,omitempty"` // 1=Finnish, 2=Swedish, 3=English
	Description  string `json:"description,omitempty"`
}

// PRHWebsite represents the company website.
type PRHWebsite struct {
	URL              string `json:"url,omitempty"`
	RegistrationDate string `json:"registrationDate,omitempty"`
	Source           string `json:"source,omitempty"`
}

// PRHCompanyForm represents the legal form with validity period.
type PRHCompanyForm struct {
	Type             string           `json:"type,omitempty"`
	Descriptions     []PRHDescription `json:"descriptions,omitempty"`
	RegistrationDate string           `json:"registrationDate,omitempty"`
	EndDate          string           `json:"endDate,omitempty"`
	Version          int              `json:"version,omitempty"`
	Source           string           `json:"source,omitempty"`
}

// PRHCompanySituation represents company situation (e.g., bankruptcy).
type PRHCompanySituation struct {
	Type             string `json:"type,omitempty"` // KONK=bankruptcy
	RegistrationDate string `json:"registrationDate,omitempty"`
	Source           string `json:"source,omitempty"`
}

// PRHRegisteredEntry represents registration status entries.
type PRHRegisteredEntry struct {
	Type             string           `json:"type,omitempty"`
	Descriptions     []PRHDescription `json:"descriptions,omitempty"`
	RegistrationDate string           `json:"registrationDate,omitempty"`
	EndDate          string           `json:"endDate,omitempty"`
	Register         string           `json:"register,omitempty"`
	Authority        string           `json:"authority,omitempty"`
}

// PRHAddress represents an address from PRH.
type PRHAddress struct {
	Type             int              `json:"type,omitempty"` // 1=visiting, 2=postal
	Street           string           `json:"street,omitempty"`
	BuildingNumber   string           `json:"buildingNumber,omitempty"`
	Entrance         string           `json:"entrance,omitempty"`
	ApartmentNumber  string           `json:"apartmentNumber,omitempty"`
	PostCode         string           `json:"postCode,omitempty"`
	PostOffices      []PRHPostOffice  `json:"postOffices,omitempty"`
	PostOfficeBox    string           `json:"postOfficeBox,omitempty"`
	CareOf           string           `json:"co,omitempty"`
	RegistrationDate string           `json:"registrationDate,omitempty"`
	EndDate          string           `json:"endDate,omitempty"`
	Source           string           `json:"source,omitempty"`
}

// PRHPostOffice represents the city/municipality.
type PRHPostOffice struct {
	City             string `json:"city,omitempty"`
	LanguageCode     string `json:"languageCode,omitempty"`
	MunicipalityCode string `json:"municipalityCode,omitempty"`
}

// PRHSearchResponse represents search results from PRH YTJ API v3.
type PRHSearchResponse struct {
	TotalResults int          `json:"totalResults"`
	Companies    []PRHCompany `json:"companies"`
}

// parseDate parses a PRH date string (YYYY-MM-DD format).
func parseDate(dateStr string) *time.Time {
	if dateStr == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil
	}
	return &t
}

// GetCurrentName returns the current (active) name of the company.
func (c *PRHCompany) GetCurrentName() string {
	// Look for an active official name (type 1)
	for _, n := range c.Names {
		if n.EndDate == "" && n.Type == "1" {
			return n.Name
		}
	}
	// Fallback to any active name
	for _, n := range c.Names {
		if n.EndDate == "" {
			return n.Name
		}
	}
	// Return first name if no active one found
	if len(c.Names) > 0 {
		return c.Names[0].Name
	}
	return ""
}

// GetCurrentAddress returns the current business address.
func (c *PRHCompany) GetCurrentAddress() *PRHAddress {
	// Prefer visiting address (type 1)
	for i := range c.Addresses {
		if c.Addresses[i].Type == 1 {
			return &c.Addresses[i]
		}
	}
	// Fallback to postal address (type 2)
	for i := range c.Addresses {
		if c.Addresses[i].Type == 2 {
			return &c.Addresses[i]
		}
	}
	if len(c.Addresses) > 0 {
		return &c.Addresses[0]
	}
	return nil
}

// GetCity returns the city from an address.
func (a *PRHAddress) GetCity() string {
	// Prefer Finnish (languageCode 1)
	for _, po := range a.PostOffices {
		if po.LanguageCode == "1" {
			return po.City
		}
	}
	if len(a.PostOffices) > 0 {
		return a.PostOffices[0].City
	}
	return ""
}

// GetCurrentCompanyForm returns the current legal form.
func (c *PRHCompany) GetCurrentCompanyForm() *PRHCompanyForm {
	for i := range c.CompanyForms {
		if c.CompanyForms[i].EndDate == "" {
			return &c.CompanyForms[i]
		}
	}
	if len(c.CompanyForms) > 0 {
		return &c.CompanyForms[0]
	}
	return nil
}

// GetFormName returns the localized form name.
func (f *PRHCompanyForm) GetFormName(lang string) string {
	for _, d := range f.Descriptions {
		if d.LanguageCode == lang {
			return d.Description
		}
	}
	// Default to English (3) or first available
	for _, d := range f.Descriptions {
		if d.LanguageCode == "3" {
			return d.Description
		}
	}
	if len(f.Descriptions) > 0 {
		return f.Descriptions[0].Description
	}
	return ""
}

// IsActive checks if the company is currently active.
func (c *PRHCompany) IsActive() bool {
	// Check tradeRegisterStatus: 1=registered, 4=ceased
	if c.TradeRegisterStatus == "1" {
		return true
	}
	if c.TradeRegisterStatus == "4" {
		return false
	}
	// Check for active registration entries
	for _, entry := range c.RegisteredEntries {
		if entry.EndDate == "" && entry.Type == "1" { // 1=registered
			return true
		}
	}
	return c.EndDate == ""
}

// IsInLiquidation checks if the company is in liquidation/bankruptcy.
func (c *PRHCompany) IsInLiquidation() bool {
	for _, sit := range c.CompanySituations {
		if sit.Type == "KONK" { // KONK = bankruptcy
			return true
		}
	}
	return false
}

// GetMainBusinessDescription returns the main business line description.
func (c *PRHCompany) GetMainBusinessDescription(lang string) string {
	if c.MainBusinessLine == nil {
		return ""
	}
	for _, d := range c.MainBusinessLine.Descriptions {
		if d.LanguageCode == lang {
			return d.Description
		}
	}
	// Default to English or first
	for _, d := range c.MainBusinessLine.Descriptions {
		if d.LanguageCode == "3" {
			return d.Description
		}
	}
	if len(c.MainBusinessLine.Descriptions) > 0 {
		return c.MainBusinessLine.Descriptions[0].Description
	}
	return ""
}
