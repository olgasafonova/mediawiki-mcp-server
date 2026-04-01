package tools

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/olgasafonova/mediawiki-mcp-server/metrics"
	"github.com/olgasafonova/mediawiki-mcp-server/tracing"
	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// HandlerRegistry provides type-safe tool registration by mapping
// tool names to their concrete handler implementations.
type HandlerRegistry struct {
	client      *wiki.Client
	logger      *slog.Logger
	auditLogger ToolAuditLogger
}

// NewHandlerRegistry creates a new handler registry.
func NewHandlerRegistry(client *wiki.Client, logger *slog.Logger) *HandlerRegistry {
	return &HandlerRegistry{
		client:      client,
		logger:      logger,
		auditLogger: NullToolAuditLogger{},
	}
}

// WithAuditLogger sets the handler-level audit logger.
func (h *HandlerRegistry) WithAuditLogger(l ToolAuditLogger) *HandlerRegistry {
	if l != nil {
		h.auditLogger = l
	}
	return h
}

// RegisterAll registers all tools with the MCP server.
func (h *HandlerRegistry) RegisterAll(server *mcp.Server) {
	for _, spec := range AllTools {
		h.registerByName(server, spec)
	}
	h.logger.Info("Registered all tools", "count", len(AllTools))
}

// registerByName dispatches to the correct typed registration function.
func (h *HandlerRegistry) registerByName(server *mcp.Server, spec ToolSpec) {
	tool := h.buildTool(spec)

	switch spec.Method {
	// Search tools
	case "Search":
		h.register(server, tool, spec, h.client.Search)
	case "SearchInPage":
		h.register(server, tool, spec, h.client.SearchInPage)
	case "SearchInFile":
		h.register(server, tool, spec, h.client.SearchInFile)
	case "ResolveTitle":
		h.register(server, tool, spec, h.client.ResolveTitle)

	// Read tools
	case "GetPage":
		h.register(server, tool, spec, h.client.GetPage)
	case "ListPages":
		h.register(server, tool, spec, h.client.ListPages)
	case "GetPageInfo":
		h.register(server, tool, spec, h.client.GetPageInfo)
	case "GetSections":
		h.register(server, tool, spec, h.client.GetSections)
	case "GetRelated":
		h.register(server, tool, spec, h.client.GetRelated)
	case "GetImages":
		h.register(server, tool, spec, h.client.GetImages)
	case "Parse":
		h.register(server, tool, spec, h.client.Parse)
	case "GetWikiInfo":
		h.register(server, tool, spec, h.client.GetWikiInfo)

	// Category tools
	case "ListCategories":
		h.register(server, tool, spec, h.client.ListCategories)
	case "GetCategoryMembers":
		h.register(server, tool, spec, h.client.GetCategoryMembers)

	// History tools
	case "GetRecentChanges":
		h.register(server, tool, spec, h.client.GetRecentChanges)
	case "GetRevisions":
		h.register(server, tool, spec, h.client.GetRevisions)
	case "CompareRevisions":
		h.register(server, tool, spec, h.client.CompareRevisions)
	case "GetUserContributions":
		h.register(server, tool, spec, h.client.GetUserContributions)

	// Link tools
	case "GetExternalLinks":
		h.register(server, tool, spec, h.client.GetExternalLinks)
	case "GetExternalLinksBatch":
		h.register(server, tool, spec, h.client.GetExternalLinksBatch)
	case "CheckLinks":
		h.register(server, tool, spec, h.client.CheckLinks)
	case "GetBacklinks":
		h.register(server, tool, spec, h.client.GetBacklinks)
	case "FindBrokenInternalLinks":
		h.register(server, tool, spec, h.client.FindBrokenInternalLinks)
	case "FindOrphanedPages":
		h.register(server, tool, spec, h.client.FindOrphanedPages)

	// Quality tools
	case "CheckTerminology":
		h.register(server, tool, spec, h.client.CheckTerminology)
	case "CheckTranslations":
		h.register(server, tool, spec, h.client.CheckTranslations)
	case "HealthAudit":
		h.register(server, tool, spec, h.client.HealthAudit)

	// Discovery tools
	case "FindSimilarPages":
		h.register(server, tool, spec, h.client.FindSimilarPages)
	case "CompareTopic":
		h.register(server, tool, spec, h.client.CompareTopic)

	// User tools
	case "ListUsers":
		h.register(server, tool, spec, h.client.ListUsers)

	// Batch tools
	case "GetPagesBatch":
		h.register(server, tool, spec, h.client.GetPagesBatch)
	case "GetPagesInfoBatch":
		h.register(server, tool, spec, h.client.GetPagesInfoBatch)

	// Composite tools
	case "SearchAndRead":
		h.register(server, tool, spec, h.client.SearchAndRead)
	case "GetPageSummary":
		h.register(server, tool, spec, h.client.GetPageSummary)

	// Page management tools
	case "MovePage":
		h.register(server, tool, spec, h.client.MovePage)
	case "ManageCategories":
		h.register(server, tool, spec, h.client.ManageCategories)

	// Wiki hygiene tools
	case "GetStalePages":
		h.register(server, tool, spec, h.client.GetStalePages)

	// Write tools
	case "EditPage":
		h.register(server, tool, spec, h.client.EditPage)
	case "FindReplace":
		h.register(server, tool, spec, h.client.FindReplace)
	case "ApplyFormatting":
		h.register(server, tool, spec, h.client.ApplyFormatting)
	case "BulkReplace":
		h.register(server, tool, spec, h.client.BulkReplace)
	case "UploadFile":
		h.register(server, tool, spec, h.client.UploadFile)

	default:
		h.logger.Error("Unknown method, tool not registered", "method", spec.Method, "tool", spec.Name)
	}
}

// buildTool creates an mcp.Tool from a ToolSpec.
func (h *HandlerRegistry) buildTool(spec ToolSpec) *mcp.Tool {
	annotations := &mcp.ToolAnnotations{
		Title:          spec.Title,
		ReadOnlyHint:   spec.ReadOnly,
		IdempotentHint: spec.Idempotent,
	}
	if spec.Destructive {
		annotations.DestructiveHint = ptr(true)
	}
	if spec.OpenWorld {
		annotations.OpenWorldHint = ptr(true)
	}

	return &mcp.Tool{
		Name:        spec.Name,
		Description: spec.Description,
		Annotations: annotations,
	}
}

// register is a generic helper that registers a tool with the MCP server.
// It wraps the client method with panic recovery, metrics, tracing, and logging.
func register[Args, Result any](
	h *HandlerRegistry,
	server *mcp.Server,
	tool *mcp.Tool,
	spec ToolSpec,
	method func(context.Context, Args) (Result, error),
) {
	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, args Args) (*mcp.CallToolResult, Result, error) {
		defer h.recoverPanic(spec.Name)

		// Start trace span
		ctx, span := tracing.StartSpan(ctx, "mcp.tool."+spec.Name)
		defer span.End()

		span.SetAttributes(
			attribute.String("mcp.tool.name", spec.Name),
			attribute.String("mcp.tool.category", spec.Category),
			attribute.Bool("mcp.tool.readonly", spec.ReadOnly),
		)

		// Track in-flight requests
		metrics.RequestInFlight.WithLabelValues(spec.Name).Inc()
		defer metrics.RequestInFlight.WithLabelValues(spec.Name).Dec()

		start := time.Now()
		result, err := method(ctx, args)
		duration := time.Since(start).Seconds()

		span.SetAttributes(attribute.Float64("mcp.tool.duration_seconds", duration))

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			metrics.RecordRequest(spec.Name, duration, false)
			h.auditLogger.Log(newToolCallEntry(spec, args, err, start))
			var zero Result
			return nil, zero, fmt.Errorf("%s failed: %w", spec.Name, err)
		}

		span.SetStatus(codes.Ok, "")
		metrics.RecordRequest(spec.Name, duration, true)
		h.logExecution(spec, args, result)
		h.auditLogger.Log(newToolCallEntry(spec, args, nil, start))
		return nil, result, nil
	})
}

// recoverPanic recovers from panics in tool handlers.
func (h *HandlerRegistry) recoverPanic(toolName string) {
	if rec := recover(); rec != nil {
		metrics.PanicsRecovered.WithLabelValues(toolName).Inc()
		h.logger.Error("Panic recovered",
			"tool", toolName,
			"panic", rec,
			"stack", string(debug.Stack()))
	}
}

// logExecution logs tool execution details.
func (h *HandlerRegistry) logExecution(spec ToolSpec, args, result any) {
	// Build log attributes from the spec
	attrs := []any{"tool", spec.Name}

	// Add any extractable fields from args/result using type assertions
	// This is more performant than reflection for common cases
	switch a := args.(type) {
	case wiki.SearchArgs:
		attrs = append(attrs, "query", a.Query)
	case wiki.GetPageArgs:
		attrs = append(attrs, "title", a.Title, "format", a.Format)
	case wiki.SearchInPageArgs:
		attrs = append(attrs, "title", a.Title, "query", a.Query)
	case wiki.EditPageArgs:
		attrs = append(attrs, "title", a.Title, "content_len", len(a.Content))
	case wiki.FindReplaceArgs:
		attrs = append(attrs, "title", a.Title, "preview", a.Preview)
	case wiki.BulkReplaceArgs:
		attrs = append(attrs, "pages_count", len(a.Pages), "preview", a.Preview)
	case wiki.GetPagesBatchArgs:
		attrs = append(attrs, "titles_count", len(a.Titles))
	case wiki.SearchAndReadArgs:
		attrs = append(attrs, "query", a.Query, "read_count", a.ReadCount)
	case wiki.GetPageSummaryArgs:
		attrs = append(attrs, "title", a.Title)
	case wiki.MovePageArgs:
		attrs = append(attrs, "from", a.From, "to", a.To)
	case wiki.ManageCategoriesArgs:
		attrs = append(attrs, "title", a.Title, "add", len(a.Add), "remove", len(a.Remove))
	case wiki.GetStalePagesArgs:
		attrs = append(attrs, "days", a.Days, "category", a.Category)
	}

	switch r := result.(type) {
	case wiki.SearchResult:
		attrs = append(attrs, "results_count", len(r.Results), "total_hits", r.TotalHits)
	case wiki.PageContent:
		attrs = append(attrs, "output_chars", len(r.Content))
	case wiki.EditResult:
		attrs = append(attrs, "success", r.Success, "new_page", r.NewPage)
	case wiki.FindReplaceResult:
		attrs = append(attrs, "matches", r.MatchCount, "replaced", r.ReplaceCount)
	case wiki.BulkReplaceResult:
		attrs = append(attrs, "pages_modified", r.PagesModified, "total_changes", r.TotalChanges)
	case wiki.GetPagesBatchResult:
		attrs = append(attrs, "found", r.FoundCount, "missing", r.MissingCount)
	case wiki.SearchAndReadResult:
		attrs = append(attrs, "total_hits", r.TotalHits, "pages_read", len(r.Pages))
	case wiki.PageSummaryResult:
		attrs = append(attrs, "sections", r.SectionCount, "length", r.Length)
	case wiki.MovePageResult:
		attrs = append(attrs, "success", r.Success, "from", r.From, "to", r.To)
	case wiki.ManageCategoriesResult:
		attrs = append(attrs, "added", len(r.Added), "removed", len(r.Removed))
	case wiki.GetStalePagesResult:
		attrs = append(attrs, "stale_count", r.StaleCount, "scanned", r.TotalScanned)
	}

	h.logger.Info("Tool executed", attrs...)
}

// Convenience function to call the generic register with method receiver
func (h *HandlerRegistry) register(server *mcp.Server, tool *mcp.Tool, spec ToolSpec, method any) {
	switch m := method.(type) {
	// Search tools
	case func(context.Context, wiki.SearchArgs) (wiki.SearchResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.SearchInPageArgs) (wiki.SearchInPageResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.SearchInFileArgs) (wiki.SearchInFileResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.ResolveTitleArgs) (wiki.ResolveTitleResult, error):
		register(h, server, tool, spec, m)

	// Read tools
	case func(context.Context, wiki.GetPageArgs) (wiki.PageContent, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.ListPagesArgs) (wiki.ListPagesResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.PageInfoArgs) (wiki.PageInfo, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.GetSectionsArgs) (wiki.GetSectionsResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.GetRelatedArgs) (wiki.GetRelatedResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.GetImagesArgs) (wiki.GetImagesResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.ParseArgs) (wiki.ParseResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.WikiInfoArgs) (wiki.WikiInfo, error):
		register(h, server, tool, spec, m)

	// Category tools
	case func(context.Context, wiki.ListCategoriesArgs) (wiki.ListCategoriesResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.CategoryMembersArgs) (wiki.CategoryMembersResult, error):
		register(h, server, tool, spec, m)

	// History tools
	case func(context.Context, wiki.RecentChangesArgs) (wiki.RecentChangesResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.GetRevisionsArgs) (wiki.GetRevisionsResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.CompareRevisionsArgs) (wiki.CompareRevisionsResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.GetUserContributionsArgs) (wiki.GetUserContributionsResult, error):
		register(h, server, tool, spec, m)

	// Link tools
	case func(context.Context, wiki.GetExternalLinksArgs) (wiki.ExternalLinksResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.GetExternalLinksBatchArgs) (wiki.ExternalLinksBatchResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.CheckLinksArgs) (wiki.CheckLinksResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.GetBacklinksArgs) (wiki.GetBacklinksResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.FindBrokenInternalLinksArgs) (wiki.FindBrokenInternalLinksResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.FindOrphanedPagesArgs) (wiki.FindOrphanedPagesResult, error):
		register(h, server, tool, spec, m)

	// Quality tools
	case func(context.Context, wiki.CheckTerminologyArgs) (wiki.CheckTerminologyResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.CheckTranslationsArgs) (wiki.CheckTranslationsResult, error):
		register(h, server, tool, spec, m)

	// Discovery tools
	case func(context.Context, wiki.FindSimilarPagesArgs) (wiki.FindSimilarPagesResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.CompareTopicArgs) (wiki.CompareTopicResult, error):
		register(h, server, tool, spec, m)

	// User tools
	case func(context.Context, wiki.ListUsersArgs) (wiki.ListUsersResult, error):
		register(h, server, tool, spec, m)

	// Batch tools
	case func(context.Context, wiki.GetPagesBatchArgs) (wiki.GetPagesBatchResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.GetPagesInfoBatchArgs) (wiki.GetPagesInfoBatchResult, error):
		register(h, server, tool, spec, m)

	// Composite tools
	case func(context.Context, wiki.SearchAndReadArgs) (wiki.SearchAndReadResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.GetPageSummaryArgs) (wiki.PageSummaryResult, error):
		register(h, server, tool, spec, m)

	// Page management tools
	case func(context.Context, wiki.MovePageArgs) (wiki.MovePageResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.ManageCategoriesArgs) (wiki.ManageCategoriesResult, error):
		register(h, server, tool, spec, m)

	// Wiki hygiene tools
	case func(context.Context, wiki.GetStalePagesArgs) (wiki.GetStalePagesResult, error):
		register(h, server, tool, spec, m)

	// Write tools
	case func(context.Context, wiki.EditPageArgs) (wiki.EditResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.FindReplaceArgs) (wiki.FindReplaceResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.ApplyFormattingArgs) (wiki.ApplyFormattingResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.BulkReplaceArgs) (wiki.BulkReplaceResult, error):
		register(h, server, tool, spec, m)
	case func(context.Context, wiki.UploadFileArgs) (wiki.UploadFileResult, error):
		register(h, server, tool, spec, m)

	default:
		h.logger.Error("Unknown method type, tool not registered", "tool", spec.Name)
	}
}
