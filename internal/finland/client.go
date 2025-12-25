package finland

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
	// DefaultBaseURL is the base URL for the PRH YTJ API (v3).
	DefaultBaseURL = "https://avoindata.prh.fi/opendata-ytj-api/v3/companies"

	// DefaultTimeout is the default HTTP timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultUserAgent identifies this client.
	DefaultUserAgent = "NordicRegistryMCP/1.0"
)

// Config holds configuration for the Finland client.
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

// Client provides access to the Finnish PRH API.
type Client struct {
	config     Config
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new Finland client.
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

// GetCompanyByID fetches a company by its Y-tunnus (business ID).
func (c *Client) GetCompanyByID(ctx context.Context, businessID string) (*PRHCompany, error) {
	// Clean and format the business ID
	cleaned := cleanBusinessID(businessID)

	// Format as 1234567-8 if needed
	if len(cleaned) == 8 {
		businessID = cleaned[:7] + "-" + cleaned[7:]
	}

	// Use search with businessId parameter
	u, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	q := u.Query()
	q.Set("businessId", businessID)
	u.RawQuery = q.Encode()

	c.logger.Debug("Fetching company", "businessID", businessID, "url", u.String())

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

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("company not found: %s", businessID)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp PRHSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(searchResp.Companies) == 0 {
		return nil, fmt.Errorf("company not found: %s", businessID)
	}

	return &searchResp.Companies[0], nil
}

// Search searches for companies by name.
func (c *Client) Search(ctx context.Context, params SearchParams) (*PRHSearchResponse, error) {
	u, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	q := u.Query()
	if params.Name != "" {
		q.Set("name", params.Name)
	}
	if params.BusinessID != "" {
		q.Set("businessId", params.BusinessID)
	}
	if params.CompanyForm != "" {
		q.Set("companyForm", params.CompanyForm)
	}
	if params.MaxResults > 0 {
		q.Set("maxResults", fmt.Sprintf("%d", params.MaxResults))
	}

	u.RawQuery = q.Encode()

	c.logger.Debug("Searching companies", "url", u.String())

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

	var result PRHSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

// SearchParams holds search parameters.
type SearchParams struct {
	Name                  string // Company name
	BusinessID            string // Y-tunnus
	CompanyForm           string // Legal form
	RegistrationDateStart string // YYYY-MM-DD
	RegistrationDateEnd   string // YYYY-MM-DD
	MaxResults            int    // Max results to return
	ResultsFrom           int    // Pagination offset
}

// cleanBusinessID removes formatting from a Finnish business ID.
func cleanBusinessID(id string) string {
	// Remove hyphen and spaces
	id = strings.ReplaceAll(id, "-", "")
	id = strings.ReplaceAll(id, " ", "")
	return strings.TrimSpace(id)
}

// ToCompany converts a PRH company to the unified Company type.
func (c *PRHCompany) ToCompany() *registry.Company {
	company := &registry.Company{
		OrgNumber:      c.BusinessID.Value,
		Country:        registry.CountryFinland,
		Name:           c.GetCurrentName(),
		SourceRegistry: "PRH",
		RawData:        c,
	}

	// Set registration date
	company.RegistrationDate = parseDate(c.RegistrationDate)

	// Get legal form
	if form := c.GetCurrentCompanyForm(); form != nil {
		company.LegalFormName = form.GetFormName("3") // English
		company.LegalFormCode = form.Type
	}

	// Determine status
	switch {
	case c.IsInLiquidation():
		company.Status = registry.StatusLiquidated
	case !c.IsActive():
		company.Status = registry.StatusInactive
	default:
		company.Status = registry.StatusActive
	}

	// Convert address
	if addr := c.GetCurrentAddress(); addr != nil {
		street := addr.Street
		if addr.BuildingNumber != "" {
			street += " " + addr.BuildingNumber
		}
		company.BusinessAddress = &registry.Address{
			Street:     street,
			PostalCode: addr.PostCode,
			City:       addr.GetCity(),
			Country:    "Finland",
		}
		if addr.CareOf != "" {
			company.BusinessAddress.Street2 = "c/o " + addr.CareOf
		}
	}

	// Convert main business line to industry code
	if c.MainBusinessLine != nil {
		company.IndustryCodes = append(company.IndustryCodes, registry.IndustryCode{
			Code:        c.MainBusinessLine.Type,
			Description: c.GetMainBusinessDescription("3"), // English
			IsPrimary:   true,
			System:      c.MainBusinessLine.TypeCodeSet,
		})
	}

	// Registered office from address
	if addr := c.GetCurrentAddress(); addr != nil {
		company.RegisteredOffice = addr.GetCity()
	}

	// Website
	if c.Website != nil {
		company.Website = c.Website.URL
	}

	return company
}

// GetCompany is a convenience method that returns a unified Company.
func (c *Client) GetCompany(ctx context.Context, businessID string) (*registry.Company, error) {
	prh, err := c.GetCompanyByID(ctx, businessID)
	if err != nil {
		return nil, err
	}
	return prh.ToCompany(), nil
}

// SearchCompanies searches and returns unified Company results.
func (c *Client) SearchCompanies(ctx context.Context, name string, limit int) (*registry.SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	resp, err := c.Search(ctx, SearchParams{
		Name:       name,
		MaxResults: limit,
	})
	if err != nil {
		return nil, err
	}

	result := &registry.SearchResult{
		TotalCount: resp.TotalResults,
		PageSize:   limit,
	}

	for _, co := range resp.Companies {
		result.Companies = append(result.Companies, *co.ToCompany())
	}

	return result, nil
}
