# Nordic Registry MCP Server - Project Plan

**Status:** Planning
**Owner:** Olga Safonova
**Target:** Production-ready enterprise MCP server for Nordic company registers
**License:** Proprietary

---

## Executive Summary

Build an MCP server providing unified access to company registry data across Norway, Denmark, Finland, and Sweden. Designed as core infrastructure for Public 360 AI integration, enabling intelligent company lookup, verification, and compliance features.

---

## 1. API Landscape

### 1.1 Norway - Brønnøysundregistrene (BRREG)

| Attribute | Value |
|-----------|-------|
| **API Base** | `https://data.brreg.no/enhetsregisteret/api` |
| **Auth** | None (fully open) |
| **Rate Limit** | Reasonable use policy |
| **License** | NLOD (Norwegian License for Open Government Data) |
| **Docs** | https://data.brreg.no/enhetsregisteret/api/dokumentasjon/en/index.html |

**Key Endpoints:**
- `GET /enheter/{orgnr}` - Get entity by org number
- `GET /enheter` - Search entities
- `GET /underenheter/{orgnr}` - Get sub-entities
- `GET /enheter/lastned` - Bulk download

**Data Available:**
- Company name, org number, type
- Address (business and postal)
- Industry codes (NACE)
- Registration date, status
- Number of employees
- Bankruptcy/liquidation status

---

### 1.2 Finland - Patent and Registration Office (PRH/YTJ)

| Attribute | Value |
|-----------|-------|
| **API Base** | `http://avoindata.prh.fi/bis/v1` |
| **Auth** | None (fully open) |
| **Rate Limit** | Reasonable use |
| **License** | Open data |
| **Docs** | https://avoindata.prh.fi/ytj_en.html |

**Key Endpoints:**
- `GET /companies/{businessId}` - Get company by Y-tunnus
- `GET /companies` - Search companies

**Data Available:**
- Company name, business ID (Y-tunnus)
- Legal form, status
- Registered addresses
- Industry codes
- Registration dates

**Limitations:**
- No private traders (toiminimi without VAT)
- No municipalities or tax partnerships
- Updated once daily

---

### 1.3 Denmark - Central Business Register (CVR)

| Attribute | Value |
|-----------|-------|
| **API Base** | `http://distribution.virk.dk/cvr-permanent` |
| **Auth** | Username/Password (free registration) |
| **Rate Limit** | TBD after registration |
| **License** | Open data with attribution |
| **Docs** | https://datacvr.virk.dk/data/?language=en-gb |

**Key Endpoints:**
- Elasticsearch-based query API
- `POST /virksomhed/_search` - Search companies

**Data Available:**
- Company name, CVR number
- Legal form, status
- Addresses
- Industry codes
- Beneficial owners (when available)
- Board members and management

**Action Required:** Register at datacvr.virk.dk for API credentials

---

### 1.4 Sweden - Bolagsverket

| Attribute | Value |
|-----------|-------|
| **API Base** | `https://portal.api.bolagsverket.se` |
| **Auth** | API key (developer portal registration) |
| **Rate Limit** | TBD |
| **License** | Open data (as of 2025) |
| **Docs** | https://bolagsverket.se/apierochoppnadata |

**Key Endpoints:**
- Company information API (företagsinformation)
- Annual reports API (årsredovisningar)

**Data Available:**
- Company name, org number
- Addresses
- Business activities
- Registration status
- Annual reports (PDF)

**Note:** Bolagsverket transitioned to free API access in 2025. Developer portal registration required.

---

## 2. Technical Architecture

### 2.1 Project Structure

```
nordic-registry-mcp-server/
├── main.go                          # Entry, transport, health endpoints
├── go.mod
├── go.sum
├── Makefile                         # Build, test, release targets
├── .goreleaser.yml                  # Cross-platform builds
│
├── internal/                        # Private packages
│   ├── registry/                    # Core domain
│   │   ├── client.go                # Multi-registry orchestrator
│   │   ├── types.go                 # Unified Company, Person types
│   │   ├── errors.go                # Structured errors
│   │   ├── config.go                # Environment configuration
│   │   ├── cache.go                 # LRU cache with TTL
│   │   ├── ratelimit.go             # Per-registry rate limiters
│   │   ├── circuitbreaker.go        # Resilience patterns
│   │   └── validation.go            # Org number validation
│   │
│   ├── norway/                      # BRREG implementation
│   │   ├── client.go
│   │   ├── types.go
│   │   ├── mapper.go                # BRREG -> unified types
│   │   └── client_test.go
│   │
│   ├── finland/                     # PRH implementation
│   │   ├── client.go
│   │   ├── types.go
│   │   ├── mapper.go
│   │   └── client_test.go
│   │
│   ├── denmark/                     # CVR implementation
│   │   ├── client.go
│   │   ├── types.go
│   │   ├── auth.go                  # Session management
│   │   ├── mapper.go
│   │   └── client_test.go
│   │
│   └── sweden/                      # Bolagsverket implementation
│       ├── client.go
│       ├── types.go
│       ├── mapper.go
│       └── client_test.go
│
├── tools/                           # MCP tools
│   ├── definitions.go               # ToolSpec metadata
│   ├── handlers.go                  # Handler registry
│   └── handlers_test.go
│
├── resources/                       # MCP resources
│   └── resources.go                 # company://{country}/{org_number}
│
├── prompts/                         # MCP prompts
│   └── prompts.go                   # Workflow templates
│
└── docs/
    ├── API_REFERENCE.md
    └── PUBLIC_360_INTEGRATION.md
```

### 2.2 Unified Data Model

```go
// Country identifies a Nordic country
type Country string

const (
    CountryNorway  Country = "NO"
    CountryDenmark Country = "DK"
    CountryFinland Country = "FI"
    CountrySweden  Country = "SE"
)

// Company represents unified company data across all registries
type Company struct {
    // Identifiers
    OrganizationNumber string  `json:"organization_number"`
    Country            Country `json:"country"`

    // Names
    Name               string   `json:"name"`
    AlternativeNames   []string `json:"alternative_names,omitempty"`

    // Legal status
    LegalForm          string `json:"legal_form"`           // AS, AB, OY, ApS, etc.
    LegalFormCode      string `json:"legal_form_code"`      // Standardized code
    Status             string `json:"status"`               // ACTIVE, DISSOLVED, BANKRUPT
    StatusCode         string `json:"status_code"`

    // Addresses
    BusinessAddress    *Address `json:"business_address,omitempty"`
    PostalAddress      *Address `json:"postal_address,omitempty"`

    // Classification
    IndustryCodes      []IndustryCode `json:"industry_codes,omitempty"`

    // Dates
    RegistrationDate   *time.Time `json:"registration_date,omitempty"`
    FoundingDate       *time.Time `json:"founding_date,omitempty"`
    DissolutionDate    *time.Time `json:"dissolution_date,omitempty"`

    // Size indicators
    NumberOfEmployees  *int `json:"number_of_employees,omitempty"`

    // People
    BoardMembers       []Person `json:"board_members,omitempty"`
    BeneficialOwners   []Person `json:"beneficial_owners,omitempty"`
    SignatoryRights    []SignatoryRight `json:"signatory_rights,omitempty"`

    // Metadata
    LastUpdated        time.Time `json:"last_updated"`
    SourceRegistry     string    `json:"source_registry"`
    SourceURL          string    `json:"source_url,omitempty"`
}

type Address struct {
    Street     string `json:"street,omitempty"`
    PostalCode string `json:"postal_code,omitempty"`
    City       string `json:"city,omitempty"`
    Country    string `json:"country,omitempty"`
}

type Person struct {
    Name        string     `json:"name"`
    BirthDate   *time.Time `json:"birth_date,omitempty"`
    Role        string     `json:"role"`
    RoleCode    string     `json:"role_code,omitempty"`
    StartDate   *time.Time `json:"start_date,omitempty"`
    EndDate     *time.Time `json:"end_date,omitempty"`
    Nationality string     `json:"nationality,omitempty"`
}

type IndustryCode struct {
    Code        string `json:"code"`
    Description string `json:"description"`
    System      string `json:"system"` // NACE, SIC, etc.
    IsPrimary   bool   `json:"is_primary"`
}
```

### 2.3 Organization Number Formats

| Country | Format | Example | Validation |
|---------|--------|---------|------------|
| Norway | 9 digits | 123456789 | MOD11 check digit |
| Denmark | 8 digits (CVR) | 12345678 | MOD11 check digit |
| Finland | 7 digits + check + hyphen | 1234567-8 | MOD11 check digit |
| Sweden | 10 digits | 1234567890 | Luhn check digit |

```go
// DetectCountry identifies country from org number format
func DetectCountry(orgNumber string) (Country, error) {
    cleaned := strings.ReplaceAll(orgNumber, " ", "")
    cleaned = strings.ReplaceAll(cleaned, "-", "")

    switch len(cleaned) {
    case 7, 8: // Finland (7+1) or Denmark (8)
        if strings.Contains(orgNumber, "-") {
            return CountryFinland, nil
        }
        return CountryDenmark, nil
    case 9:
        return CountryNorway, nil
    case 10:
        return CountrySweden, nil
    default:
        return "", fmt.Errorf("unrecognized org number format: %s", orgNumber)
    }
}
```

---

## 3. MCP Tool Definitions

### 3.1 Core Tools

```go
var AllTools = []ToolSpec{
    // =========================================================================
    // Cross-Country Search
    // =========================================================================
    {
        Name:        "nordic_search_company",
        Method:      "SearchCompany",
        Title:       "Search Nordic Companies",
        Category:    "search",
        ReadOnly:    true,
        Description: `Search for companies across Nordic countries by name.

USE WHEN: User asks "find companies named X", "search for X in Norway"

PARAMETERS:
- query: Company name to search for (required)
- countries: Filter by countries ["NO", "DK", "FI", "SE"] (optional, default all)
- limit: Max results per country (default 10, max 50)

RETURNS: List of companies with org numbers, names, status, and country.`,
    },

    // =========================================================================
    // Company Lookup
    // =========================================================================
    {
        Name:        "nordic_get_company",
        Method:      "GetCompany",
        Title:       "Get Company Details",
        Category:    "lookup",
        ReadOnly:    true,
        Description: `Get detailed company information by organization number.
Auto-detects country from org number format.

USE WHEN: User asks "get details for org 123456789", "company info for X"

PARAMETERS:
- org_number: Organization number (required)
- country: Override auto-detection ["NO", "DK", "FI", "SE"] (optional)
- include_people: Include board/owners (default true)
- include_history: Include historical changes (default false)

RETURNS: Full company details including name, address, status, industry, people.`,
    },

    {
        Name:        "nordic_get_company_status",
        Method:      "GetCompanyStatus",
        Title:       "Check Company Status",
        Category:    "verification",
        ReadOnly:    true,
        Description: `Quick status check for a company - is it active, dissolved, or bankrupt?

USE WHEN: User asks "is company X active?", "check if 123456789 exists"

PARAMETERS:
- org_number: Organization number (required)

RETURNS: Status (ACTIVE/DISSOLVED/BANKRUPT/UNKNOWN), name, last updated.`,
    },

    // =========================================================================
    // People and Ownership
    // =========================================================================
    {
        Name:        "nordic_get_board_members",
        Method:      "GetBoardMembers",
        Title:       "Get Board Members",
        Category:    "people",
        ReadOnly:    true,
        Description: `Get current board members and management for a company.

USE WHEN: User asks "who is on the board of X", "board members for 123456789"

PARAMETERS:
- org_number: Organization number (required)

RETURNS: List of board members with names, roles, and dates.`,
    },

    {
        Name:        "nordic_get_beneficial_owners",
        Method:      "GetBeneficialOwners",
        Title:       "Get Beneficial Owners",
        Category:    "people",
        ReadOnly:    true,
        Description: `Get Ultimate Beneficial Owners (UBO) for a company.
Note: UBO data availability varies by country and company type.

USE WHEN: User asks "who owns X", "beneficial owners of 123456789"

PARAMETERS:
- org_number: Organization number (required)

RETURNS: List of beneficial owners with names and ownership percentages.`,
    },

    {
        Name:        "nordic_get_signatory_rights",
        Method:      "GetSignatoryRights",
        Title:       "Get Signatory Rights",
        Category:    "people",
        ReadOnly:    true,
        Description: `Get who can legally sign on behalf of a company.

USE WHEN: User asks "who can sign for X", "signatory rights for 123456789"

PARAMETERS:
- org_number: Organization number (required)

RETURNS: Signatory rights information (varies by country).`,
    },

    // =========================================================================
    // Validation
    // =========================================================================
    {
        Name:        "nordic_validate_org_number",
        Method:      "ValidateOrgNumber",
        Title:       "Validate Organization Number",
        Category:    "validation",
        ReadOnly:    true,
        Description: `Validate organization number format and check digit.
Does NOT verify the company exists - only validates the number format.

USE WHEN: User asks "is this org number valid", "check format of 123456789"

PARAMETERS:
- org_number: Organization number to validate (required)

RETURNS: Valid (bool), detected country, formatted number.`,
    },

    // =========================================================================
    // Country-Specific (Power User)
    // =========================================================================
    {
        Name:        "norway_get_enheter",
        Method:      "NorwayGetEnheter",
        Title:       "Norway: Get Entity",
        Category:    "norway",
        ReadOnly:    true,
        Description: `Get Norwegian entity directly from BRREG Enhetsregisteret.

PARAMETERS:
- orgnr: 9-digit organization number (required)

RETURNS: Raw BRREG entity data.`,
    },

    {
        Name:        "norway_search_enheter",
        Method:      "NorwaySearchEnheter",
        Title:       "Norway: Search Entities",
        Category:    "norway",
        ReadOnly:    true,
        Description: `Search Norwegian entities in BRREG.

PARAMETERS:
- navn: Company name
- organisasjonsform: Legal form code (AS, ENK, etc.)
- kommunenummer: Municipality code
- size: Results per page (max 100)

RETURNS: List of matching entities.`,
    },

    // Similar for finland_, denmark_, sweden_ prefixed tools...
}
```

### 3.2 MCP Resources

```go
// Resources provide direct access via URIs
// Format: company://{country}/{org_number}

server.AddResourceTemplate(&mcp.ResourceTemplate{
    URITemplate: "company://{country}/{org_number}",
    Name:        "Nordic Company",
    Description: "Get company data by country code and org number. Example: company://NO/123456789",
    MIMEType:    "application/json",
}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
    // Parse URI and fetch company
})
```

### 3.3 MCP Prompts

```go
// Prompts for common workflows
var AllPrompts = []PromptSpec{
    {
        Name:        "verify-counterparty",
        Description: "Verify a business counterparty before signing contracts",
        Template: `Please verify the following company:
Organization number: {{org_number}}

Check:
1. Is the company active and not dissolved/bankrupt?
2. Who are the board members?
3. Who can sign on behalf of the company?
4. What is their registered business address?

Provide a summary suitable for contract due diligence.`,
    },

    {
        Name:        "compare-companies",
        Description: "Compare two companies side by side",
        Template: `Compare these two companies:
Company 1: {{org_number_1}}
Company 2: {{org_number_2}}

Show:
- Basic info (name, status, legal form)
- Size (employees if available)
- Industry
- Key differences`,
    },

    {
        Name:        "ownership-chain",
        Description: "Trace ownership chain for a company",
        Template: `Trace the ownership structure for:
Organization number: {{org_number}}

Show:
- Beneficial owners
- Parent companies (if any)
- Ownership percentages`,
    },
}
```

---

## 4. Enterprise Features

### 4.1 Configuration

```go
type Config struct {
    // Registry-specific settings
    Norway  NorwayConfig
    Denmark DenmarkConfig
    Finland FinlandConfig
    Sweden  SwedenConfig

    // General settings
    Timeout      time.Duration
    MaxRetries   int
    UserAgent    string

    // Caching
    CacheEnabled bool
    CacheTTL     time.Duration
    MaxCacheSize int

    // Rate limiting
    RateLimitEnabled bool

    // Audit
    AuditLogPath string
    AuditEnabled bool

    // HTTP transport
    HTTPAddr       string
    BearerToken    string
    AllowedOrigins []string
    RateLimit      int
    TrustedProxies []string
}

type DenmarkConfig struct {
    Username string // CVR API username
    Password string // CVR API password
}

type SwedenConfig struct {
    APIKey string // Bolagsverket API key
}
```

**Environment Variables:**

```bash
# Norway (no auth needed)
# BRREG_BASE_URL=https://data.brreg.no  # optional override

# Denmark
CVR_USERNAME=your_username
CVR_PASSWORD=your_password

# Finland (no auth needed)
# PRH_BASE_URL=http://avoindata.prh.fi  # optional override

# Sweden
BOLAGSVERKET_API_KEY=your_api_key

# General
NORDIC_TIMEOUT=30s
NORDIC_CACHE_TTL=5m
NORDIC_AUDIT_LOG=/var/log/nordic-registry/audit.log

# HTTP transport
NORDIC_HTTP_ADDR=:8080
NORDIC_AUTH_TOKEN=your_bearer_token
NORDIC_ALLOWED_ORIGINS=https://public360.example.com
NORDIC_RATE_LIMIT=60
```

### 4.2 Rate Limiting Strategy

```go
// Per-registry rate limits (conservative defaults)
var DefaultRateLimits = map[Country]RateLimitConfig{
    CountryNorway: {
        RequestsPerSecond: 10,
        BurstSize:         20,
    },
    CountryDenmark: {
        RequestsPerSecond: 5,
        BurstSize:         10,
    },
    CountryFinland: {
        RequestsPerSecond: 5,
        BurstSize:         10,
    },
    CountrySweden: {
        RequestsPerSecond: 5,
        BurstSize:         10,
    },
}
```

### 4.3 Caching Strategy

```go
// Cache TTLs by data type
var CacheTTLs = map[string]time.Duration{
    "company":         5 * time.Minute,   // Company details
    "search":          1 * time.Minute,   // Search results
    "status":          2 * time.Minute,   // Status checks
    "people":          5 * time.Minute,   // Board/owners
    "validation":      24 * time.Hour,    // Org number validation (static)
}
```

### 4.4 Audit Logging

```go
type AuditEvent struct {
    Timestamp   time.Time `json:"timestamp"`
    Tool        string    `json:"tool"`
    OrgNumber   string    `json:"org_number,omitempty"`
    Country     Country   `json:"country,omitempty"`
    Query       string    `json:"query,omitempty"`
    Success     bool      `json:"success"`
    Error       string    `json:"error,omitempty"`
    DurationMs  int64     `json:"duration_ms"`
    UserID      string    `json:"user_id,omitempty"`   // From Public 360
    SessionID   string    `json:"session_id,omitempty"`
}
```

---

## 5. Public 360 Integration Strategy

### 5.1 Integration Model

**Dual positioning:**
- **B: Nordic Business Intelligence Add-on** - Standalone product for P360 customers
- **C: Core AI Infrastructure** - Powers P360's AI/intelligent features

### 5.2 Integration Points

```
┌─────────────────────────────────────────────────────────────────┐
│                        Public 360                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────┐    ┌─────────────────┐    ┌──────────────┐ │
│  │  Case Parties   │    │  Contract Mgmt  │    │   Search     │ │
│  │  Management     │    │                 │    │              │ │
│  └────────┬────────┘    └────────┬────────┘    └──────┬───────┘ │
│           │                      │                     │         │
│           └──────────────────────┼─────────────────────┘         │
│                                  │                               │
│                    ┌─────────────▼─────────────┐                 │
│                    │   P360 Integration Layer   │                │
│                    │   (REST API / GraphQL)     │                │
│                    └─────────────┬─────────────┘                 │
│                                  │                               │
└──────────────────────────────────┼───────────────────────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │  Nordic Registry MCP Server  │
                    │  (stdio or HTTP transport)   │
                    └──────────────┬──────────────┘
                                   │
          ┌────────────┬───────────┼───────────┬────────────┐
          │            │           │           │            │
     ┌────▼────┐  ┌────▼────┐ ┌────▼────┐ ┌────▼────┐       │
     │  BRREG  │  │   CVR   │ │   PRH   │ │Bolagsv. │       │
     │ Norway  │  │ Denmark │ │ Finland │ │ Sweden  │       │
     └─────────┘  └─────────┘ └─────────┘ └─────────┘       │
```

### 5.3 Use Cases for Public 360

| Use Case | Description | P360 Feature |
|----------|-------------|--------------|
| **Auto-fill Case Party** | When adding a company to a case, auto-populate name/address from registry | Case Management |
| **Verify Counterparty** | Before contract signing, verify company is active and not bankrupt | Contract Management |
| **Board Member Lookup** | Find authorized signatories for a company | Contract Management |
| **Compliance Check** | Ensure business partners are legitimate entities | Compliance Module |
| **Search Enhancement** | "Find cases for Equinor subsidiaries" - resolve company names to org numbers | Global Search |
| **Change Alerts** | Notify when a company in active cases changes status | Monitoring |

### 5.4 Integration Options

**Option 1: Direct MCP Integration**
If P360 has MCP client support, connect directly via stdio or HTTP.

**Option 2: REST API Wrapper**
Build a thin REST API layer over the MCP server for traditional integration.

```
P360 → REST API → Nordic Registry MCP Server → Registries
```

**Option 3: Event-Driven**
Publish company data changes to a message queue (Azure Service Bus, etc.) for P360 consumption.

### 5.5 Licensing Model Ideas

| Tier | Features | Price Model |
|------|----------|-------------|
| **Basic** | Norway only, 1000 lookups/month | Per user/month |
| **Nordic** | All 4 countries, 10000 lookups/month | Per user/month |
| **Enterprise** | Unlimited, audit logs, SLA, change alerts | Annual contract |

---

## 6. Development Phases

### Phase 1: Foundation (Week 1-2)
- [ ] Project scaffold with Go module
- [ ] Unified data types
- [ ] Norway (BRREG) client - fully working
- [ ] Finland (PRH) client - fully working
- [ ] Core tools: search, get_company, validate
- [ ] stdio transport
- [ ] Unit tests (>80% coverage)
- [ ] Basic README

### Phase 2: Full Nordic (Week 3-4)
- [ ] Register for CVR API credentials
- [ ] Register for Bolagsverket developer portal
- [ ] Denmark (CVR) client with auth
- [ ] Sweden (Bolagsverket) client with auth
- [ ] Cross-country search unification
- [ ] HTTP transport with security
- [ ] Health endpoints
- [ ] Integration tests

### Phase 3: Enterprise Features (Week 5-6)
- [ ] Audit logging
- [ ] Prometheus metrics
- [ ] Circuit breakers
- [ ] Board/UBO/Signatory tools
- [ ] MCP Resources
- [ ] MCP Prompts
- [ ] Performance optimization
- [ ] Documentation

### Phase 4: Public 360 Integration (Week 7-8)
- [ ] P360 integration layer design
- [ ] REST API wrapper (if needed)
- [ ] Authentication with P360
- [ ] Pilot customer testing
- [ ] Production deployment guide

---

## 7. Testing Strategy

### 7.1 Unit Tests
- All mapper functions
- Org number validation
- Cache operations
- Rate limiting logic

### 7.2 Integration Tests
- Live API calls to each registry (sandbox/test org numbers)
- Error handling for network failures
- Rate limit behavior

### 7.3 Test Org Numbers

| Country | Test Org Number | Name |
|---------|-----------------|------|
| Norway | 923609016 | BRREG test entity |
| Denmark | 10150817 | DR (Danmarks Radio) |
| Finland | 0112038-9 | Oy Hartwall Ab |
| Sweden | 5560125790 | Volvo AB |

---

## 8. Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Bolagsverket API requires paid tier | High | Use third-party API (Allabolag) or web scraping fallback |
| CVR API credentials rejected | Medium | Start with Norway+Finland, add Denmark later |
| Rate limiting too aggressive | Medium | Implement adaptive rate limiting, respect headers |
| Data format changes | Medium | Versioned mappers, comprehensive tests |
| GDPR concerns with person data | High | Only return data from official public registers |

---

## 9. Success Metrics

- **Reliability:** 99.9% uptime for cached queries
- **Performance:** <500ms p95 for single company lookup
- **Coverage:** All 4 Nordic countries operational
- **Adoption:** Integration into P360 within 3 months
- **Quality:** >80% test coverage

---

## 10. Next Steps

1. **Create project repository** (private, under TietoEvry org)
2. **Register for APIs:**
   - [ ] CVR Denmark: https://datacvr.virk.dk
   - [ ] Bolagsverket Sweden: https://portal.api.bolagsverket.se
3. **Implement Phase 1** - Norway and Finland clients
4. **Schedule P360 integration discussion** with product team

---

## Appendix A: API Response Examples

### Norway BRREG Response

```json
{
  "organisasjonsnummer": "923609016",
  "navn": "REGISTERENHETEN I BRØNNØYSUND",
  "organisasjonsform": {
    "kode": "ORGL",
    "beskrivelse": "Organisasjonsledd"
  },
  "registreringsdatoEnhetsregisteret": "2009-08-17",
  "forretningsadresse": {
    "land": "Norge",
    "landkode": "NO",
    "postnummer": "8910",
    "poststed": "BRØNNØYSUND",
    "adresse": ["Havnegata 48"],
    "kommune": "BRØNNØY",
    "kommunenummer": "1813"
  },
  "naeringskode1": {
    "kode": "84.110",
    "beskrivelse": "Generell offentlig administrasjon"
  }
}
```

### Finland PRH Response

```json
{
  "businessId": "0112038-9",
  "name": "Oy Hartwall Ab",
  "registrationDate": "1979-05-21",
  "companyForm": "OY",
  "detailsUri": "http://avoindata.prh.fi/bis/v1/0112038-9",
  "businessLines": [
    {
      "code": "11070",
      "name": "Soft drinks manufacturing",
      "language": "EN"
    }
  ],
  "addresses": [
    {
      "street": "Kimmeltie 3",
      "postCode": "02770",
      "city": "ESPOO",
      "type": 1
    }
  ]
}
```

---

*Document created: 2025-01-17*
*Last updated: 2025-01-17*
