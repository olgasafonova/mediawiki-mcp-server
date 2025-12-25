// Package registry provides unified data types for Nordic company registries.
package registry

import "time"

// Country represents a Nordic country.
type Country string

const (
	CountryNorway  Country = "NO"
	CountryDenmark Country = "DK"
	CountryFinland Country = "FI"
	CountrySweden  Country = "SE"
)

// String returns the country code.
func (c Country) String() string {
	return string(c)
}

// Name returns the full country name.
func (c Country) Name() string {
	switch c {
	case CountryNorway:
		return "Norway"
	case CountryDenmark:
		return "Denmark"
	case CountryFinland:
		return "Finland"
	case CountrySweden:
		return "Sweden"
	default:
		return "Unknown"
	}
}

// CompanyStatus represents the operational status of a company.
type CompanyStatus string

const (
	StatusActive     CompanyStatus = "active"
	StatusInactive   CompanyStatus = "inactive"
	StatusDissolved  CompanyStatus = "dissolved"
	StatusBankrupt   CompanyStatus = "bankrupt"
	StatusLiquidated CompanyStatus = "liquidated"
	StatusUnknown    CompanyStatus = "unknown"
)

// Company represents a unified company record across Nordic registries.
type Company struct {
	// Core identifiers
	OrgNumber     string  `json:"org_number"`
	Country       Country `json:"country"`
	Name          string  `json:"name"`
	LegalFormCode string  `json:"legal_form_code,omitempty"`
	LegalFormName string  `json:"legal_form_name,omitempty"`

	// Status
	Status           CompanyStatus `json:"status"`
	RegistrationDate *time.Time    `json:"registration_date,omitempty"`
	DissolutionDate  *time.Time    `json:"dissolution_date,omitempty"`

	// Address
	BusinessAddress  *Address `json:"business_address,omitempty"`
	PostalAddress    *Address `json:"postal_address,omitempty"`
	RegisteredOffice string   `json:"registered_office,omitempty"`

	// Industry
	IndustryCodes []IndustryCode `json:"industry_codes,omitempty"`

	// Employees (when available)
	Employees *int `json:"employees,omitempty"`

	// Contact info
	Website string `json:"website,omitempty"`

	// Source metadata
	SourceRegistry string     `json:"source_registry"`
	LastUpdated    *time.Time `json:"last_updated,omitempty"`
	RawData        any        `json:"raw_data,omitempty"` // Original registry data
}

// Address represents a physical or postal address.
type Address struct {
	Street     string `json:"street,omitempty"`
	Street2    string `json:"street2,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	City       string `json:"city,omitempty"`
	Country    string `json:"country,omitempty"`
	County     string `json:"county,omitempty"`     // Fylke (NO), Region (DK), etc.
	Municipal  string `json:"municipal,omitempty"`  // Kommune
}

// IsEmpty returns true if the address has no content.
func (a *Address) IsEmpty() bool {
	if a == nil {
		return true
	}
	return a.Street == "" && a.City == "" && a.PostalCode == ""
}

// Format returns a formatted single-line address.
func (a *Address) Format() string {
	if a == nil || a.IsEmpty() {
		return ""
	}
	result := a.Street
	if a.Street2 != "" {
		result += ", " + a.Street2
	}
	if a.PostalCode != "" || a.City != "" {
		result += ", " + a.PostalCode + " " + a.City
	}
	if a.Country != "" {
		result += ", " + a.Country
	}
	return result
}

// IndustryCode represents a business activity classification.
type IndustryCode struct {
	Code        string `json:"code"`
	Description string `json:"description,omitempty"`
	IsPrimary   bool   `json:"is_primary,omitempty"`
	System      string `json:"system,omitempty"` // NACE, SNI, etc.
}

// Person represents a person associated with a company.
type Person struct {
	Name        string     `json:"name"`
	BirthDate   *time.Time `json:"birth_date,omitempty"`
	BirthYear   *int       `json:"birth_year,omitempty"`
	Nationality string     `json:"nationality,omitempty"`
	Address     *Address   `json:"address,omitempty"`
	PersonID    string     `json:"person_id,omitempty"` // Country-specific ID
}

// Role represents a person's role in a company.
type Role struct {
	Type      string     `json:"type"`      // board_member, ceo, accountant, auditor, etc.
	Title     string     `json:"title"`     // Localized title
	Person    *Person    `json:"person,omitempty"`
	Company   *Company   `json:"company,omitempty"` // When role holder is a company
	StartDate *time.Time `json:"start_date,omitempty"`
	EndDate   *time.Time `json:"end_date,omitempty"`
}

// BoardMember is an alias for a board role.
type BoardMember struct {
	Role
	Position string `json:"position,omitempty"` // Chair, Deputy Chair, Member
}

// Owner represents ownership information.
type Owner struct {
	Person        *Person  `json:"person,omitempty"`
	Company       *Company `json:"company,omitempty"`
	SharePercent  float64  `json:"share_percent,omitempty"`
	VotingPercent float64  `json:"voting_percent,omitempty"`
	IsUBO         bool     `json:"is_ubo,omitempty"` // Ultimate Beneficial Owner
}

// SignatoryRights represents who can sign on behalf of the company.
type SignatoryRights struct {
	Description string   `json:"description,omitempty"`
	Signatories []Person `json:"signatories,omitempty"`
	Rules       string   `json:"rules,omitempty"` // e.g., "Two board members jointly"
}

// SearchResult represents a company search result.
type SearchResult struct {
	Companies  []Company `json:"companies"`
	TotalCount int       `json:"total_count"`
	Page       int       `json:"page,omitempty"`
	PageSize   int       `json:"page_size,omitempty"`
}

// ValidationResult represents org number validation.
type ValidationResult struct {
	Valid           bool    `json:"valid"`
	OrgNumber       string  `json:"org_number"`
	FormattedNumber string  `json:"formatted_number"`
	Country         Country `json:"country,omitempty"`
	CheckDigitValid bool    `json:"check_digit_valid"`
	Message         string  `json:"message,omitempty"`
}
