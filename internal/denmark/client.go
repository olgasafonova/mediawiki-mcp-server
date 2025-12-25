package denmark

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/olgasafonova/nordic-registry-mcp-server/internal/registry"
)

const (
	// DefaultBaseURL is the base URL for the cvrapi.dk API.
	DefaultBaseURL = "https://cvrapi.dk/api"

	// DefaultTimeout is the default HTTP timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultUserAgent identifies this client.
	DefaultUserAgent = "NordicRegistryMCP/1.0"
)

// Config holds configuration for the Denmark client.
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

// Client provides access to the Danish CVR API.
type Client struct {
	config     Config
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new Denmark client.
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

// GetCompanyByCVR fetches a company by its CVR number.
func (c *Client) GetCompanyByCVR(ctx context.Context, cvr string) (*CVRCompany, error) {
	// Clean CVR number
	cleaned := cleanCVR(cvr)

	u, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	q := u.Query()
	q.Set("vat", cleaned)
	q.Set("country", "dk")
	u.RawQuery = q.Encode()

	c.logger.Debug("Fetching company", "cvr", cleaned, "url", u.String())

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
		return nil, fmt.Errorf("company not found: %s", cvr)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var company CVRCompany
	if err := json.NewDecoder(resp.Body).Decode(&company); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Check for API error response
	if company.VAT == 0 {
		return nil, fmt.Errorf("company not found: %s", cvr)
	}

	return &company, nil
}

// Search searches for companies by name.
func (c *Client) Search(ctx context.Context, name string) (*CVRCompany, error) {
	u, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	q := u.Query()
	q.Set("search", name)
	q.Set("country", "dk")
	u.RawQuery = q.Encode()

	c.logger.Debug("Searching companies", "name", name, "url", u.String())

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

	// cvrapi.dk returns a single company for search, not a list
	var company CVRCompany
	if err := json.NewDecoder(resp.Body).Decode(&company); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if company.VAT == 0 {
		return nil, fmt.Errorf("no companies found matching: %s", name)
	}

	return &company, nil
}

// cleanCVR removes formatting from a Danish CVR number.
func cleanCVR(cvr string) string {
	result := ""
	for _, ch := range cvr {
		if ch >= '0' && ch <= '9' {
			result += string(ch)
		}
	}
	return result
}

// ToCompany converts a CVR company to the unified Company type.
func (c *CVRCompany) ToCompany() *registry.Company {
	company := &registry.Company{
		OrgNumber:      strconv.Itoa(c.VAT),
		Country:        registry.CountryDenmark,
		Name:           c.Name,
		LegalFormCode:  strconv.Itoa(c.CompanyCode),
		LegalFormName:  c.CompanyDesc,
		SourceRegistry: "CVR",
		RawData:        c,
	}

	// Set registration date
	company.RegistrationDate = parseDate(c.StartDate)

	// Set dissolution date if ended
	if c.EndDate != nil {
		company.DissolutionDate = parseDate(*c.EndDate)
	}

	// Determine status
	switch {
	case c.CreditBankrupt:
		company.Status = registry.StatusBankrupt
	case c.EndDate != nil:
		company.Status = registry.StatusInactive
	default:
		company.Status = registry.StatusActive
	}

	// Convert address
	if c.Address != "" || c.City != "" {
		company.BusinessAddress = &registry.Address{
			Street:     c.Address,
			PostalCode: c.Zipcode,
			City:       c.City,
			Country:    "Denmark",
		}
		if c.AddressCO != "" {
			company.BusinessAddress.Street2 = "c/o " + c.AddressCO
		}
	}

	// Convert industry code
	if c.IndustryCode > 0 {
		company.IndustryCodes = append(company.IndustryCodes, registry.IndustryCode{
			Code:        strconv.Itoa(c.IndustryCode),
			Description: c.IndustryDesc,
			IsPrimary:   true,
			System:      "NACE",
		})
	}

	// Employees
	company.Employees = c.Employees

	// Registered office from city
	company.RegisteredOffice = c.City

	// Phone as contact info (no website from this API)
	// Note: cvrapi.dk doesn't provide website info

	return company
}

// GetCompany is a convenience method that returns a unified Company.
func (c *Client) GetCompany(ctx context.Context, cvr string) (*registry.Company, error) {
	cvrCompany, err := c.GetCompanyByCVR(ctx, cvr)
	if err != nil {
		return nil, err
	}
	return cvrCompany.ToCompany(), nil
}

// SearchCompanies searches and returns unified Company results.
// Note: cvrapi.dk free tier only returns one result per search.
func (c *Client) SearchCompanies(ctx context.Context, name string, limit int) (*registry.SearchResult, error) {
	company, err := c.Search(ctx, name)
	if err != nil {
		return nil, err
	}

	result := &registry.SearchResult{
		TotalCount: 1,
		PageSize:   1,
	}
	result.Companies = append(result.Companies, *company.ToCompany())

	return result, nil
}
