package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/olgasafonova/nordic-registry-mcp-server/internal/denmark"
	"github.com/olgasafonova/nordic-registry-mcp-server/internal/finland"
	"github.com/olgasafonova/nordic-registry-mcp-server/internal/norway"
	"github.com/olgasafonova/nordic-registry-mcp-server/internal/registry"
)

// Registry manages MCP tool handlers.
type Registry struct {
	norwayClient  *norway.Client
	finlandClient *finland.Client
	denmarkClient *denmark.Client
	logger        *slog.Logger
}

// NewRegistry creates a new tool registry.
func NewRegistry(norwayClient *norway.Client, finlandClient *finland.Client, denmarkClient *denmark.Client, logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{
		norwayClient:  norwayClient,
		finlandClient: finlandClient,
		denmarkClient: denmarkClient,
		logger:        logger,
	}
}

// RegisterAll registers all tools with the MCP server.
func (r *Registry) RegisterAll(server *mcp.Server) {
	for _, spec := range AllTools {
		r.registerTool(server, spec)
	}
	r.logger.Debug("Registered MCP tools", "count", len(AllTools))
}

func (r *Registry) registerTool(server *mcp.Server, spec ToolSpec) {
	// Build input schema as map
	properties := make(map[string]any)
	required := []string{}

	for _, param := range spec.Parameters {
		prop := map[string]any{
			"type":        param.Type,
			"description": param.Description,
		}
		if len(param.Enum) > 0 {
			prop["enum"] = param.Enum
		}
		properties[param.Name] = prop
		if param.Required {
			required = append(required, param.Name)
		}
	}

	inputSchema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		inputSchema["required"] = required
	}

	server.AddTool(&mcp.Tool{
		Name:        spec.Name,
		Description: spec.Description,
		InputSchema: inputSchema,
	}, r.getHandler(spec.Name))
}

func (r *Registry) getHandler(name string) mcp.ToolHandler {
	switch name {
	case "nordic_search":
		return r.handleNordicSearch
	case "nordic_get_company":
		return r.handleNordicGetCompany
	case "nordic_get_status":
		return r.handleNordicGetStatus
	case "nordic_get_board":
		return r.handleNordicGetBoard
	case "nordic_validate_org_number":
		return r.handleNordicValidateOrgNumber
	case "norway_get_enhet":
		return r.handleNorwayGetEnhet
	case "norway_search":
		return r.handleNorwaySearch
	case "finland_get_company":
		return r.handleFinlandGetCompany
	case "finland_search":
		return r.handleFinlandSearch
	case "denmark_get_company":
		return r.handleDenmarkGetCompany
	case "denmark_search":
		return r.handleDenmarkSearch
	case "nordic_batch_lookup":
		return r.handleNordicBatchLookup
	case "norway_get_ownership":
		return r.handleNorwayGetOwnership
	case "norway_get_subsidiaries":
		return r.handleNorwayGetSubsidiaries
	default:
		return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, fmt.Errorf("unknown tool: %s", name)
		}
	}
}

// parseArguments unmarshals the raw JSON arguments into a map.
func parseArguments(req *mcp.CallToolRequest) (map[string]any, error) {
	if len(req.Params.Arguments) == 0 {
		return make(map[string]any), nil
	}
	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}
	return args, nil
}

// Handler implementations

func (r *Registry) handleNordicSearch(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	query, _ := args["query"].(string)
	if query == "" {
		return errorResult("query is required")
	}

	countryCode, _ := args["country"].(string)
	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	var allResults []registry.Company
	var errors []string

	// Search in specified country or all countries
	countries := []registry.Country{registry.CountryNorway, registry.CountryFinland, registry.CountryDenmark}
	if countryCode != "" {
		countries = []registry.Country{registry.Country(countryCode)}
	}

	for _, country := range countries {
		switch country {
		case registry.CountryNorway:
			if r.norwayClient != nil {
				result, err := r.norwayClient.SearchCompanies(ctx, query, limit)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Norway: %v", err))
				} else {
					allResults = append(allResults, result.Companies...)
				}
			}
		case registry.CountryFinland:
			if r.finlandClient != nil {
				result, err := r.finlandClient.SearchCompanies(ctx, query, limit)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Finland: %v", err))
				} else {
					allResults = append(allResults, result.Companies...)
				}
			}
		case registry.CountryDenmark:
			if r.denmarkClient != nil {
				result, err := r.denmarkClient.SearchCompanies(ctx, query, limit)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Denmark: %v", err))
				} else {
					allResults = append(allResults, result.Companies...)
				}
			}
		}
	}

	result := map[string]any{
		"companies": allResults,
		"count":     len(allResults),
	}
	if len(errors) > 0 {
		result["errors"] = errors
	}

	return jsonResult(result)
}

func (r *Registry) handleNordicGetCompany(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	orgNumber, _ := args["org_number"].(string)
	if orgNumber == "" {
		return errorResult("org_number is required")
	}

	countryCode, _ := args["country"].(string)

	// Detect country if not provided
	var country registry.Country
	if countryCode != "" {
		country = registry.Country(countryCode)
	} else {
		country = registry.DetectCountry(orgNumber)
		if country == "" {
			return errorResult("Could not detect country from org number. Please provide country parameter (NO, DK, FI, SE)")
		}
	}

	var company *registry.Company

	switch country {
	case registry.CountryNorway:
		if r.norwayClient != nil {
			company, err = r.norwayClient.GetCompany(ctx, orgNumber)
		} else {
			return errorResult("Norway client not configured")
		}
	case registry.CountryFinland:
		if r.finlandClient != nil {
			company, err = r.finlandClient.GetCompany(ctx, orgNumber)
		} else {
			return errorResult("Finland client not configured")
		}
	case registry.CountryDenmark:
		if r.denmarkClient != nil {
			company, err = r.denmarkClient.GetCompany(ctx, orgNumber)
		} else {
			return errorResult("Denmark client not configured")
		}
	case registry.CountrySweden:
		return errorResult("Sweden not yet implemented")
	default:
		return errorResult("Unknown country: " + string(country))
	}

	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get company: %v", err))
	}

	return jsonResult(company)
}

func (r *Registry) handleNordicGetStatus(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	orgNumber, _ := args["org_number"].(string)
	if orgNumber == "" {
		return errorResult("org_number is required")
	}

	countryCode, _ := args["country"].(string)

	var country registry.Country
	if countryCode != "" {
		country = registry.Country(countryCode)
	} else {
		country = registry.DetectCountry(orgNumber)
		if country == "" {
			return errorResult("Could not detect country from org number")
		}
	}

	var company *registry.Company

	switch country {
	case registry.CountryNorway:
		if r.norwayClient != nil {
			company, err = r.norwayClient.GetCompany(ctx, orgNumber)
		}
	case registry.CountryFinland:
		if r.finlandClient != nil {
			company, err = r.finlandClient.GetCompany(ctx, orgNumber)
		}
	case registry.CountryDenmark:
		if r.denmarkClient != nil {
			company, err = r.denmarkClient.GetCompany(ctx, orgNumber)
		}
	default:
		return errorResult("Country not yet implemented: " + string(country))
	}

	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get company: %v", err))
	}

	result := map[string]any{
		"org_number": company.OrgNumber,
		"name":       company.Name,
		"status":     company.Status,
		"country":    company.Country,
	}

	return jsonResult(result)
}

func (r *Registry) handleNordicGetBoard(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	orgNumber, _ := args["org_number"].(string)
	if orgNumber == "" {
		return errorResult("org_number is required")
	}

	countryCode, _ := args["country"].(string)

	var country registry.Country
	if countryCode != "" {
		country = registry.Country(countryCode)
	} else {
		country = registry.DetectCountry(orgNumber)
		if country == "" {
			return errorResult("Could not detect country from org number")
		}
	}

	switch country {
	case registry.CountryNorway:
		if r.norwayClient != nil {
			members, err := r.norwayClient.GetBoardMembers(ctx, orgNumber)
			if err != nil {
				return errorResult(fmt.Sprintf("Failed to get board: %v", err))
			}
			return jsonResult(map[string]any{
				"org_number": orgNumber,
				"country":    country,
				"members":    members,
				"count":      len(members),
			})
		}
		return errorResult("Norway client not configured")
	default:
		return errorResult("Board lookup not yet implemented for: " + string(country))
	}
}

func (r *Registry) handleNordicValidateOrgNumber(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	orgNumber, _ := args["org_number"].(string)
	if orgNumber == "" {
		return errorResult("org_number is required")
	}

	countryCode, _ := args["country"].(string)

	var country registry.Country
	if countryCode != "" {
		country = registry.Country(countryCode)
	} else {
		country = registry.DetectCountry(orgNumber)
	}

	if country == "" {
		// Can't validate without knowing country
		return jsonResult(map[string]any{
			"valid":      false,
			"org_number": orgNumber,
			"message":    "Could not detect country. Please provide country parameter.",
			"detected":   false,
		})
	}

	result := registry.ValidateOrgNumber(orgNumber, country)
	return jsonResult(result)
}

func (r *Registry) handleNorwayGetEnhet(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	orgnr, _ := args["orgnr"].(string)
	if orgnr == "" {
		return errorResult("orgnr is required")
	}

	if r.norwayClient == nil {
		return errorResult("Norway client not configured")
	}

	enhet, err := r.norwayClient.GetEnhet(ctx, orgnr)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get entity: %v", err))
	}

	return jsonResult(enhet)
}

func (r *Registry) handleNorwaySearch(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	navn, _ := args["navn"].(string)
	if navn == "" {
		return errorResult("navn is required")
	}

	if r.norwayClient == nil {
		return errorResult("Norway client not configured")
	}

	params := norway.SearchParams{
		Navn: navn,
	}
	if orgform, ok := args["organisasjonsform"].(string); ok {
		params.Organisasjonsform = orgform
	}
	if komm, ok := args["kommunenummer"].(string); ok {
		params.Kommunenummer = komm
	}
	if size, ok := args["size"].(float64); ok {
		params.Size = int(size)
	}

	result, err := r.norwayClient.SearchEnheter(ctx, params)
	if err != nil {
		return errorResult(fmt.Sprintf("Search failed: %v", err))
	}

	return jsonResult(result)
}

func (r *Registry) handleFinlandGetCompany(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	businessID, _ := args["business_id"].(string)
	if businessID == "" {
		return errorResult("business_id is required")
	}

	if r.finlandClient == nil {
		return errorResult("Finland client not configured")
	}

	company, err := r.finlandClient.GetCompanyByID(ctx, businessID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get company: %v", err))
	}

	return jsonResult(company)
}

func (r *Registry) handleFinlandSearch(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	name, _ := args["name"].(string)
	if name == "" {
		return errorResult("name is required")
	}

	if r.finlandClient == nil {
		return errorResult("Finland client not configured")
	}

	params := finland.SearchParams{
		Name: name,
	}
	if maxResults, ok := args["max_results"].(float64); ok {
		params.MaxResults = int(maxResults)
	}

	result, err := r.finlandClient.Search(ctx, params)
	if err != nil {
		return errorResult(fmt.Sprintf("Search failed: %v", err))
	}

	return jsonResult(result)
}

func (r *Registry) handleDenmarkGetCompany(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	cvr, _ := args["cvr"].(string)
	if cvr == "" {
		return errorResult("cvr is required")
	}

	if r.denmarkClient == nil {
		return errorResult("Denmark client not configured")
	}

	company, err := r.denmarkClient.GetCompanyByCVR(ctx, cvr)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get company: %v", err))
	}

	return jsonResult(company)
}

func (r *Registry) handleDenmarkSearch(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	name, _ := args["name"].(string)
	if name == "" {
		return errorResult("name is required")
	}

	if r.denmarkClient == nil {
		return errorResult("Denmark client not configured")
	}

	company, err := r.denmarkClient.Search(ctx, name)
	if err != nil {
		return errorResult(fmt.Sprintf("Search failed: %v", err))
	}

	return jsonResult(company)
}

func (r *Registry) handleNordicBatchLookup(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	orgNumbers, _ := args["org_numbers"].(string)
	if orgNumbers == "" {
		return errorResult("org_numbers is required")
	}

	countryCode, _ := args["country"].(string)

	// Split org numbers (preserve original format for country detection)
	rawNumbers := splitByComma(orgNumbers)
	if len(rawNumbers) == 0 {
		return errorResult("No valid organization numbers provided")
	}
	if len(rawNumbers) > 20 {
		return errorResult("Maximum 20 organization numbers allowed")
	}

	var results []map[string]any
	var errors []string

	for _, rawOrgNum := range rawNumbers {
		// Detect country BEFORE cleaning (Finnish format needs hyphen)
		var country registry.Country
		if countryCode != "" {
			country = registry.Country(countryCode)
		} else {
			country = registry.DetectCountry(rawOrgNum)
		}

		// Now clean for lookup
		orgNum := registry.CleanOrgNumber(rawOrgNum)
		if orgNum == "" {
			continue
		}

		result := map[string]any{
			"org_number": rawOrgNum,
		}

		if country == "" {
			result["error"] = "Could not detect country"
			results = append(results, result)
			continue
		}

		result["country"] = country

		var company *registry.Company
		var err error

		switch country {
		case registry.CountryNorway:
			if r.norwayClient != nil {
				company, err = r.norwayClient.GetCompany(ctx, orgNum)
			} else {
				err = fmt.Errorf("Norway client not configured")
			}
		case registry.CountryFinland:
			if r.finlandClient != nil {
				// Finland API expects formatted number (with hyphen)
				company, err = r.finlandClient.GetCompany(ctx, rawOrgNum)
			} else {
				err = fmt.Errorf("Finland client not configured")
			}
		case registry.CountryDenmark:
			if r.denmarkClient != nil {
				company, err = r.denmarkClient.GetCompany(ctx, orgNum)
			} else {
				err = fmt.Errorf("Denmark client not configured")
			}
		default:
			err = fmt.Errorf("Country not supported: %s", country)
		}

		if err != nil {
			result["error"] = err.Error()
			errors = append(errors, fmt.Sprintf("%s: %v", rawOrgNum, err))
		} else {
			result["company"] = company
		}

		results = append(results, result)
	}

	response := map[string]any{
		"results":      results,
		"total":        len(rawNumbers),
		"success":      len(rawNumbers) - len(errors),
		"failed":       len(errors),
	}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	return jsonResult(response)
}

func (r *Registry) handleNorwayGetOwnership(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	orgnr, _ := args["orgnr"].(string)
	if orgnr == "" {
		return errorResult("orgnr is required")
	}

	if r.norwayClient == nil {
		return errorResult("Norway client not configured")
	}

	roles, err := r.norwayClient.GetRoller(ctx, orgnr)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get ownership: %v", err))
	}

	return jsonResult(roles)
}

func (r *Registry) handleNorwayGetSubsidiaries(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := parseArguments(req)
	if err != nil {
		return errorResult(err.Error())
	}

	orgnr, _ := args["orgnr"].(string)
	if orgnr == "" {
		return errorResult("orgnr is required")
	}

	if r.norwayClient == nil {
		return errorResult("Norway client not configured")
	}

	subsidiaries, err := r.norwayClient.GetUnderenheter(ctx, orgnr)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get subsidiaries: %v", err))
	}

	// Convert to unified company format
	var companies []registry.Company
	for _, sub := range subsidiaries {
		companies = append(companies, *sub.ToCompany())
	}

	return jsonResult(map[string]any{
		"parent_org_number": orgnr,
		"subsidiaries":      companies,
		"count":             len(companies),
	})
}

// splitOrgNumbers splits a comma-separated list of org numbers and cleans them.
func splitOrgNumbers(input string) []string {
	parts := make([]string, 0)
	for _, part := range splitByComma(input) {
		cleaned := registry.CleanOrgNumber(part)
		if cleaned != "" {
			parts = append(parts, cleaned)
		}
	}
	return parts
}

// splitByComma splits a string by comma, trimming whitespace.
func splitByComma(s string) []string {
	var result []string
	for _, part := range splitString(s, ',') {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitString splits a string by a delimiter.
func splitString(s string, delim rune) []string {
	var result []string
	var current []rune
	for _, r := range s {
		if r == delim {
			result = append(result, string(current))
			current = nil
		} else {
			current = append(current, r)
		}
	}
	result = append(result, string(current))
	return result
}

// trimSpace removes leading and trailing whitespace.
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// Helper functions

func jsonResult(data any) (*mcp.CallToolResult, error) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(jsonBytes),
			},
		},
	}, nil
}

func errorResult(message string) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: message,
			},
		},
		IsError: true,
	}, nil
}
