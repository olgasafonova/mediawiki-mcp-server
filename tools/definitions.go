// Package tools provides MCP tool definitions and handlers for the Nordic Registry server.
package tools

// ToolSpec defines a tool's metadata for registration.
type ToolSpec struct {
	Name        string
	Title       string
	Description string
	Category    string
	ReadOnly    bool
	Parameters  []ParameterSpec
}

// ParameterSpec defines a tool parameter.
type ParameterSpec struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Enum        []string
}

// AllTools defines all available MCP tools.
var AllTools = []ToolSpec{
	// =========================================================================
	// Search Tools
	// =========================================================================
	{
		Name:     "nordic_search",
		Title:    "Search Nordic Companies",
		Category: "search",
		ReadOnly: true,
		Description: `Search for companies across all Nordic registries.

USE WHEN: User asks "find company X", "search for X", "look up X in Norway/Denmark/Finland/Sweden"

PARAMETERS:
- query: Company name to search for (required)
- country: Country code to limit search (NO, DK, FI, SE) - optional, searches all if not specified
- limit: Max results per country (default 10, max 50)

RETURNS: List of matching companies with basic info.`,
		Parameters: []ParameterSpec{
			{Name: "query", Type: "string", Description: "Company name to search for", Required: true},
			{Name: "country", Type: "string", Description: "Country code (NO, DK, FI, SE)", Required: false, Enum: []string{"NO", "DK", "FI", "SE"}},
			{Name: "limit", Type: "integer", Description: "Max results (default 10)", Required: false},
		},
	},

	// =========================================================================
	// Lookup Tools
	// =========================================================================
	{
		Name:     "nordic_get_company",
		Title:    "Get Company Details",
		Category: "lookup",
		ReadOnly: true,
		Description: `Get detailed company information by organization number.

USE WHEN: User provides an org number like "923609016" or "get company 123456789"

PARAMETERS:
- org_number: Organization number (required)
- country: Country code (NO, DK, FI, SE) - auto-detected if not provided

RETURNS: Full company details including name, status, address, industry codes.`,
		Parameters: []ParameterSpec{
			{Name: "org_number", Type: "string", Description: "Organization number", Required: true},
			{Name: "country", Type: "string", Description: "Country code (auto-detected if not provided)", Required: false, Enum: []string{"NO", "DK", "FI", "SE"}},
		},
	},

	{
		Name:     "nordic_get_status",
		Title:    "Get Company Status",
		Category: "lookup",
		ReadOnly: true,
		Description: `Check if a company is active, dissolved, or bankrupt.

USE WHEN: User asks "is company X still active", "check status of X"

PARAMETERS:
- org_number: Organization number (required)
- country: Country code (auto-detected if not provided)

RETURNS: Current operational status.`,
		Parameters: []ParameterSpec{
			{Name: "org_number", Type: "string", Description: "Organization number", Required: true},
			{Name: "country", Type: "string", Description: "Country code", Required: false, Enum: []string{"NO", "DK", "FI", "SE"}},
		},
	},

	// =========================================================================
	// People Tools
	// =========================================================================
	{
		Name:     "nordic_get_board",
		Title:    "Get Board Members",
		Category: "people",
		ReadOnly: true,
		Description: `Get board of directors for a company.

USE WHEN: User asks "who's on the board of X", "board members of 123456789"

PARAMETERS:
- org_number: Organization number (required)
- country: Country code (auto-detected if not provided)

RETURNS: List of board members with their positions.`,
		Parameters: []ParameterSpec{
			{Name: "org_number", Type: "string", Description: "Organization number", Required: true},
			{Name: "country", Type: "string", Description: "Country code", Required: false, Enum: []string{"NO", "DK", "FI", "SE"}},
		},
	},

	// =========================================================================
	// Validation Tools
	// =========================================================================
	{
		Name:     "nordic_validate_org_number",
		Title:    "Validate Organization Number",
		Category: "validation",
		ReadOnly: true,
		Description: `Validate organization number format and check digit.
Does NOT verify the company exists - only validates the number format.

USE WHEN: User asks "is this org number valid", "check format of 123456789"

PARAMETERS:
- org_number: Organization number to validate (required)
- country: Country code (auto-detected if not provided)

RETURNS: Valid (bool), detected country, formatted number.`,
		Parameters: []ParameterSpec{
			{Name: "org_number", Type: "string", Description: "Organization number to validate", Required: true},
			{Name: "country", Type: "string", Description: "Country code (optional)", Required: false, Enum: []string{"NO", "DK", "FI", "SE"}},
		},
	},

	// =========================================================================
	// Country-Specific Tools (Norway)
	// =========================================================================
	{
		Name:     "norway_get_enhet",
		Title:    "Norway: Get Entity",
		Category: "norway",
		ReadOnly: true,
		Description: `Get Norwegian entity directly from BRREG Enhetsregisteret.

PARAMETERS:
- orgnr: 9-digit organization number (required)

RETURNS: Raw BRREG entity data.`,
		Parameters: []ParameterSpec{
			{Name: "orgnr", Type: "string", Description: "9-digit organization number", Required: true},
		},
	},

	{
		Name:     "norway_search",
		Title:    "Norway: Search Entities",
		Category: "norway",
		ReadOnly: true,
		Description: `Search Norwegian entities in BRREG.

PARAMETERS:
- navn: Company name (required)
- organisasjonsform: Legal form code (AS, ENK, etc.)
- kommunenummer: Municipality code
- size: Results per page (max 100)

RETURNS: List of matching entities.`,
		Parameters: []ParameterSpec{
			{Name: "navn", Type: "string", Description: "Company name", Required: true},
			{Name: "organisasjonsform", Type: "string", Description: "Legal form code (AS, ENK, etc.)", Required: false},
			{Name: "kommunenummer", Type: "string", Description: "Municipality code", Required: false},
			{Name: "size", Type: "integer", Description: "Results per page (max 100)", Required: false},
		},
	},

	// =========================================================================
	// Country-Specific Tools (Finland)
	// =========================================================================
	{
		Name:     "finland_get_company",
		Title:    "Finland: Get Company",
		Category: "finland",
		ReadOnly: true,
		Description: `Get Finnish company directly from PRH BIS.

PARAMETERS:
- business_id: Y-tunnus (e.g., 1234567-8) (required)

RETURNS: Raw PRH company data.`,
		Parameters: []ParameterSpec{
			{Name: "business_id", Type: "string", Description: "Y-tunnus (e.g., 1234567-8)", Required: true},
		},
	},

	{
		Name:     "finland_search",
		Title:    "Finland: Search Companies",
		Category: "finland",
		ReadOnly: true,
		Description: `Search Finnish companies in PRH BIS.

PARAMETERS:
- name: Company name (required)
- max_results: Maximum results (default 20)

RETURNS: List of matching companies.`,
		Parameters: []ParameterSpec{
			{Name: "name", Type: "string", Description: "Company name", Required: true},
			{Name: "max_results", Type: "integer", Description: "Maximum results", Required: false},
		},
	},

	// =========================================================================
	// Country-Specific Tools (Denmark)
	// =========================================================================
	{
		Name:     "denmark_get_company",
		Title:    "Denmark: Get Company",
		Category: "denmark",
		ReadOnly: true,
		Description: `Get Danish company directly from CVR.

PARAMETERS:
- cvr: CVR number (8 digits) (required)

RETURNS: Raw CVR company data.`,
		Parameters: []ParameterSpec{
			{Name: "cvr", Type: "string", Description: "CVR number (8 digits)", Required: true},
		},
	},

	{
		Name:     "denmark_search",
		Title:    "Denmark: Search Companies",
		Category: "denmark",
		ReadOnly: true,
		Description: `Search Danish companies in CVR.
Note: The free API returns only the top matching result.

PARAMETERS:
- name: Company name (required)

RETURNS: Matching company data.`,
		Parameters: []ParameterSpec{
			{Name: "name", Type: "string", Description: "Company name", Required: true},
		},
	},
}

// GetToolByName returns a tool spec by name.
func GetToolByName(name string) *ToolSpec {
	for _, tool := range AllTools {
		if tool.Name == name {
			return &tool
		}
	}
	return nil
}
