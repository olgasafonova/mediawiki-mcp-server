package norway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/olgasafonova/nordic-registry-mcp-server/internal/registry"
)

const (
	// DefaultBaseURL is the base URL for the BRREG API.
	DefaultBaseURL = "https://data.brreg.no"

	// EnhetsregisteretPath is the path for the entity registry API.
	EnhetsregisteretPath = "/enhetsregisteret/api/enheter"

	// UnderenheterPath is the path for sub-units (branches).
	UnderenheterPath = "/enhetsregisteret/api/underenheter"

	// DefaultTimeout is the default HTTP timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultUserAgent identifies this client.
	DefaultUserAgent = "NordicRegistryMCP/1.0"
)

// Config holds configuration for the Norway client.
type Config struct {
	BaseURL   string
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		BaseURL:   DefaultBaseURL,
		Timeout:   DefaultTimeout,
		UserAgent: DefaultUserAgent,
	}
}

// Client provides access to the Norwegian BRREG API.
type Client struct {
	config     Config
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new Norway client.
func NewClient(config Config, logger *slog.Logger) *Client {
	if config.BaseURL == "" {
		config.BaseURL = DefaultBaseURL
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeout
	}
	if config.UserAgent == "" {
		config.UserAgent = DefaultUserAgent
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger: logger,
	}
}

// GetEnhet fetches a single entity by organization number.
func (c *Client) GetEnhet(ctx context.Context, orgNumber string) (*BRREGEnhet, error) {
	cleaned := registry.CleanOrgNumber(orgNumber)

	// Validate format
	if len(cleaned) != 9 {
		return nil, fmt.Errorf("invalid Norwegian org number: must be 9 digits, got %d", len(cleaned))
	}

	url := fmt.Sprintf("%s%s/%s", c.config.BaseURL, EnhetsregisteretPath, cleaned)

	c.logger.Debug("Fetching entity", "orgNumber", cleaned, "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("entity not found: %s", orgNumber)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var enhet BRREGEnhet
	if err := json.NewDecoder(resp.Body).Decode(&enhet); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &enhet, nil
}

// SearchEnheter searches for entities.
func (c *Client) SearchEnheter(ctx context.Context, params SearchParams) (*BRREGSearchResponse, error) {
	u, err := url.Parse(c.config.BaseURL + EnhetsregisteretPath)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	q := u.Query()
	if params.Navn != "" {
		q.Set("navn", params.Navn)
	}
	if params.Organisasjonsform != "" {
		q.Set("organisasjonsform", params.Organisasjonsform)
	}
	if params.Kommunenummer != "" {
		q.Set("kommunenummer", params.Kommunenummer)
	}
	if params.Naeringskode != "" {
		q.Set("naeringskode", params.Naeringskode)
	}
	if params.Size > 0 {
		q.Set("size", fmt.Sprintf("%d", params.Size))
	} else {
		q.Set("size", "20") // Default
	}
	if params.Page > 0 {
		q.Set("page", fmt.Sprintf("%d", params.Page))
	}
	if params.Konkurs {
		q.Set("konkurs", "true")
	}
	if params.Registert {
		q.Set("registrertIForetaksregisteret", "true")
	}

	u.RawQuery = q.Encode()

	c.logger.Debug("Searching entities", "url", u.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result BRREGSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

// GetRoller fetches roles (board members, etc.) for an entity.
func (c *Client) GetRoller(ctx context.Context, orgNumber string) (*BRREGRollerResponse, error) {
	cleaned := registry.CleanOrgNumber(orgNumber)

	url := fmt.Sprintf("%s%s/%s/roller", c.config.BaseURL, EnhetsregisteretPath, cleaned)

	c.logger.Debug("Fetching roles", "orgNumber", cleaned, "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("roles not found for: %s", orgNumber)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result BRREGRollerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

// SearchParams holds search parameters.
type SearchParams struct {
	Navn              string // Company name
	Organisasjonsform string // Legal form code (AS, ENK, etc.)
	Kommunenummer     string // Municipality code
	Naeringskode      string // Industry code (NACE)
	Size              int    // Results per page (max 100)
	Page              int    // Page number (0-based)
	Konkurs           bool   // Only bankrupt companies
	Registert         bool   // Only registered in company register
}

// ToCompany converts a BRREG entity to the unified Company type.
func (e *BRREGEnhet) ToCompany() *registry.Company {
	company := &registry.Company{
		OrgNumber:      e.Organisasjonsnummer,
		Country:        registry.CountryNorway,
		Name:           e.Navn,
		LegalFormCode:  e.Organisasjonsform.Kode,
		LegalFormName:  e.Organisasjonsform.Beskrivelse,
		SourceRegistry: "BRREG",
		RawData:        e,
	}

	// Determine status
	switch {
	case e.Konkurs:
		company.Status = registry.StatusBankrupt
	case e.UnderAvvikling:
		company.Status = registry.StatusLiquidated
	case e.UnderTvangsavviklingEllerTvangsopplosning:
		company.Status = registry.StatusDissolved
	case e.Slettedato != "":
		company.Status = registry.StatusInactive
	default:
		company.Status = registry.StatusActive
	}

	// Parse dates
	company.RegistrationDate = parseDate(e.Registreringsdato)
	if company.RegistrationDate == nil {
		company.RegistrationDate = parseDate(e.Stiftelsesdato)
	}
	company.DissolutionDate = parseDate(e.Slettedato)

	// Convert addresses
	if e.Forretningsadresse != nil {
		company.BusinessAddress = convertAddress(e.Forretningsadresse)
	}
	if e.Postadresse != nil {
		company.PostalAddress = convertAddress(e.Postadresse)
	}

	// Convert industry codes
	if e.Naeringskode1 != nil {
		company.IndustryCodes = append(company.IndustryCodes, registry.IndustryCode{
			Code:        e.Naeringskode1.Kode,
			Description: e.Naeringskode1.Beskrivelse,
			IsPrimary:   true,
			System:      "NACE",
		})
	}
	if e.Naeringskode2 != nil {
		company.IndustryCodes = append(company.IndustryCodes, registry.IndustryCode{
			Code:        e.Naeringskode2.Kode,
			Description: e.Naeringskode2.Beskrivelse,
			System:      "NACE",
		})
	}
	if e.Naeringskode3 != nil {
		company.IndustryCodes = append(company.IndustryCodes, registry.IndustryCode{
			Code:        e.Naeringskode3.Kode,
			Description: e.Naeringskode3.Beskrivelse,
			System:      "NACE",
		})
	}

	// Employees
	company.Employees = e.AntallAnsatte

	// Registered office from business address
	if e.Forretningsadresse != nil {
		company.RegisteredOffice = e.Forretningsadresse.Kommune
	}

	return company
}

// convertAddress converts a BRREG address to the unified Address type.
func convertAddress(addr *BRREGAdresse) *registry.Address {
	if addr == nil {
		return nil
	}

	result := &registry.Address{
		PostalCode: addr.Postnummer,
		City:       addr.Poststed,
		Country:    addr.Land,
		Municipal:  addr.Kommune,
	}

	// Join address lines
	if len(addr.Adresse) > 0 {
		result.Street = addr.Adresse[0]
		if len(addr.Adresse) > 1 {
			result.Street2 = strings.Join(addr.Adresse[1:], ", ")
		}
	}

	return result
}

// GetCompany is a convenience method that returns a unified Company.
func (c *Client) GetCompany(ctx context.Context, orgNumber string) (*registry.Company, error) {
	enhet, err := c.GetEnhet(ctx, orgNumber)
	if err != nil {
		return nil, err
	}
	return enhet.ToCompany(), nil
}

// SearchCompanies searches and returns unified Company results.
func (c *Client) SearchCompanies(ctx context.Context, name string, limit int) (*registry.SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	resp, err := c.SearchEnheter(ctx, SearchParams{
		Navn: name,
		Size: limit,
	})
	if err != nil {
		return nil, err
	}

	result := &registry.SearchResult{
		TotalCount: resp.Page.TotalElements,
		Page:       resp.Page.Number,
		PageSize:   resp.Page.Size,
	}

	for _, enhet := range resp.Embedded.Enheter {
		result.Companies = append(result.Companies, *enhet.ToCompany())
	}

	return result, nil
}

// GetBoardMembers fetches board members for an entity.
func (c *Client) GetBoardMembers(ctx context.Context, orgNumber string) ([]registry.BoardMember, error) {
	roles, err := c.GetRoller(ctx, orgNumber)
	if err != nil {
		return nil, err
	}

	var members []registry.BoardMember

	for _, group := range roles.Rollegrupper {
		// Look for board-related role groups
		if group.Type.Kode == "STYR" || group.Type.Kode == "DAGL" {
			for _, rolle := range group.Roller {
				if rolle.Fratraadt {
					continue // Skip resigned roles
				}

				member := registry.BoardMember{
					Position: rolle.Type.Beskrivelse,
				}
				member.Type = "board_member"
				member.Title = rolle.Type.Beskrivelse

				if rolle.Person != nil {
					member.Person = &registry.Person{
						Name:      rolle.Person.Navn.FullName(),
						BirthDate: parseDate(rolle.Person.Fodselsdato),
					}
				}

				if rolle.Enhet != nil {
					member.Company = &registry.Company{
						OrgNumber: rolle.Enhet.Organisasjonsnummer,
						Name:      rolle.Enhet.Organisasjonsnavn,
						Country:   registry.CountryNorway,
					}
				}

				members = append(members, member)
			}
		}
	}

	return members, nil
}
