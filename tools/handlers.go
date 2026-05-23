package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// methodRegistrar binds a ToolSpec.Method name to a closure that registers
// the matching typed handler. Each closure carries its method-specific Args
// and Result types via Go generic type inference on register[Args, Result].
type methodRegistrar func(*HandlerRegistry, *mcp.Server, *mcp.Tool, ToolSpec)

// methodRegistrars dispatches ToolSpec.Method to the typed registration
// closure. Grouped by category to mirror tools/definitions.go.
var methodRegistrars = map[string]methodRegistrar{
	// Search tools
	"Search": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.Search)
	},
	"SearchInPage": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.SearchInPage)
	},
	"SearchInFile": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.SearchInFile)
	},
	"ResolveTitle": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.ResolveTitle)
	},

	// Read tools
	"GetPage": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetPage)
	},
	"ListPages": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.ListPages)
	},
	"GetPageInfo": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetPageInfo)
	},
	"GetSections": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetSections)
	},
	"GetRelated": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetRelated)
	},
	"GetImages": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetImages)
	},
	"Parse": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.Parse)
	},
	"GetWikiInfo": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetWikiInfo)
	},

	// Category tools
	"ListCategories": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.ListCategories)
	},
	"GetCategoryMembers": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetCategoryMembers)
	},

	// History tools
	"GetRecentChanges": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetRecentChanges)
	},
	"GetRevisions": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetRevisions)
	},
	"CompareRevisions": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.CompareRevisions)
	},
	"GetUserContributions": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetUserContributions)
	},

	// Link tools
	"GetExternalLinks": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetExternalLinks)
	},
	"GetExternalLinksBatch": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetExternalLinksBatch)
	},
	"CheckLinks": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.CheckLinks)
	},
	"GetBacklinks": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetBacklinks)
	},
	"FindBrokenInternalLinks": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.FindBrokenInternalLinks)
	},
	"FindOrphanedPages": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.FindOrphanedPages)
	},

	// Quality tools
	"CheckTerminology": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.CheckTerminology)
	},
	"CheckTranslations": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.CheckTranslations)
	},
	"HealthAudit": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.HealthAudit)
	},

	// Discovery tools
	"FindSimilarPages": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.FindSimilarPages)
	},
	"CompareTopic": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.CompareTopic)
	},

	// User tools
	"ListUsers": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.ListUsers)
	},

	// Batch tools
	"GetPagesBatch": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetPagesBatch)
	},
	"GetPagesInfoBatch": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetPagesInfoBatch)
	},

	// Composite tools
	"SearchAndRead": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.SearchAndRead)
	},
	"GetPageSummary": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetPageSummary)
	},

	// Page management tools
	"MovePage": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.MovePage)
	},
	"ManageCategories": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.ManageCategories)
	},

	// Wiki hygiene tools
	"GetStalePages": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.GetStalePages)
	},

	// Write tools
	"EditPage": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.EditPage)
	},
	"FindReplace": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.FindReplace)
	},
	"ApplyFormatting": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.ApplyFormatting)
	},
	"BulkReplace": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.BulkReplace)
	},
	"UploadFile": func(h *HandlerRegistry, s *mcp.Server, t *mcp.Tool, sp ToolSpec) {
		register(h, s, t, sp, h.client.UploadFile)
	},
}

// registerByName looks up the registrar for spec.Method and invokes it.
func (h *HandlerRegistry) registerByName(server *mcp.Server, spec ToolSpec) {
	tool := h.buildTool(spec)
	if r, ok := methodRegistrars[spec.Method]; ok {
		r(h, server, tool, spec)
		return
	}
	h.logger.Error("Unknown method, tool not registered", "method", spec.Method, "tool", spec.Name)
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

// register wraps a typed client method with panic recovery, metrics, tracing,
// and logging, then attaches it to the MCP server.
//
// The dispatcher closure uses NAMED return values so the deferred recoverPanic
// can reassign `err` on panic. Without named returns, Go cannot mutate the
// return values from a deferred function and a panic-then-recover would surface
// as `(nil, zero, nil)` to the MCP caller — looking like a successful empty
// response. This is especially dangerous for destructive write tools (EditPage,
// BulkReplace, MovePage) where masked-failure-as-success is silently destructive.
// See HG-1 in rules/code-review-prompts.md.
func register[Args, Result any](
	h *HandlerRegistry,
	server *mcp.Server,
	tool *mcp.Tool,
	spec ToolSpec,
	method func(context.Context, Args) (Result, error),
) {
	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, args Args) (res *mcp.CallToolResult, out Result, err error) {
		defer h.recoverPanic(spec.Name, &err)

		ctx, span := tracing.StartSpan(ctx, "mcp.tool."+spec.Name)
		defer span.End()

		span.SetAttributes(
			attribute.String("mcp.tool.name", spec.Name),
			attribute.String("mcp.tool.category", spec.Category),
			attribute.Bool("mcp.tool.readonly", spec.ReadOnly),
			attribute.String("mcp.tool.rationale", extractRationale(args)),
		)

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

// recoverPanic recovers from panics in tool handlers and converts them into a
// structured error with a correlation ID. The panic value and stack are logged
// server-side; only the correlation ID reaches the MCP caller.
//
// MUST be called as `defer h.recoverPanic(spec.Name, &err)` from a function
// with NAMED return values. Without named returns the deferred reassignment
// is a no-op and panics surface as silent fake-success responses — a HIGH
// severity failure mode for destructive write tools (EditPage, BulkReplace,
// MovePage) where masked-failure-as-success is silently destructive.
func (h *HandlerRegistry) recoverPanic(toolName string, errPtr *error) {
	rec := recover()
	if rec == nil {
		return
	}
	corrID := newCorrelationID()
	metrics.PanicsRecovered.WithLabelValues(toolName).Inc()
	h.logger.Error("Panic recovered",
		"tool", toolName,
		"correlation_id", corrID,
		"panic", rec,
		"stack", string(debug.Stack()))
	if errPtr != nil {
		*errPtr = fmt.Errorf("%s: internal error (correlation_id=%s)", toolName, corrID)
	}
}

// newCorrelationID returns a short hex string for log correlation. Falls back
// to a timestamp-based ID if crypto/rand is unavailable (vanishingly rare).
func newCorrelationID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("ts-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// logExecution logs tool execution details. Rationale is always logged
// (empty string when absent) to keep the call branch-free; this matches
// the BaseArgs schema requirement.
func (h *HandlerRegistry) logExecution(spec ToolSpec, args, result any) {
	attrs := []any{"tool", spec.Name, "rationale", extractRationale(args)}
	attrs = appendArgAttrs(attrs, args)
	attrs = appendResultAttrs(attrs, result)
	h.logger.Info("Tool executed", attrs...)
}

// appendArgAttrs adds tool-specific argument attributes to attrs.
// Type-asserted over reflection for performance on the hot path.
func appendArgAttrs(attrs []any, args any) []any {
	switch a := args.(type) {
	case wiki.SearchArgs:
		return append(attrs, "query", a.Query)
	case wiki.GetPageArgs:
		return append(attrs, "title", a.Title, "format", a.Format)
	case wiki.SearchInPageArgs:
		return append(attrs, "title", a.Title, "query", a.Query)
	case wiki.EditPageArgs:
		return append(attrs, "title", a.Title, "content_len", len(a.Content))
	case wiki.FindReplaceArgs:
		return append(attrs, "title", a.Title, "preview", a.Preview)
	case wiki.BulkReplaceArgs:
		return append(attrs, "pages_count", len(a.Pages), "preview", a.Preview)
	case wiki.GetPagesBatchArgs:
		return append(attrs, "titles_count", len(a.Titles))
	case wiki.SearchAndReadArgs:
		return append(attrs, "query", a.Query, "read_count", a.ReadCount)
	case wiki.GetPageSummaryArgs:
		return append(attrs, "title", a.Title)
	case wiki.MovePageArgs:
		return append(attrs, "from", a.From, "to", a.To)
	case wiki.ManageCategoriesArgs:
		return append(attrs, "title", a.Title, "add", len(a.Add), "remove", len(a.Remove))
	case wiki.GetStalePagesArgs:
		return append(attrs, "days", a.Days, "category", a.Category)
	}
	return attrs
}

// appendResultAttrs adds tool-specific result attributes to attrs.
func appendResultAttrs(attrs []any, result any) []any {
	switch r := result.(type) {
	case wiki.SearchResult:
		return append(attrs, "results_count", len(r.Results), "total_hits", r.TotalHits)
	case wiki.PageContent:
		return append(attrs, "output_chars", len(r.Content))
	case wiki.EditResult:
		return append(attrs, "success", r.Success, "new_page", r.NewPage)
	case wiki.FindReplaceResult:
		return append(attrs, "matches", r.MatchCount, "replaced", r.ReplaceCount)
	case wiki.BulkReplaceResult:
		return append(attrs, "pages_modified", r.PagesModified, "total_changes", r.TotalChanges)
	case wiki.GetPagesBatchResult:
		return append(attrs, "found", r.FoundCount, "missing", r.MissingCount)
	case wiki.SearchAndReadResult:
		return append(attrs, "total_hits", r.TotalHits, "pages_read", len(r.Pages))
	case wiki.PageSummaryResult:
		return append(attrs, "sections", r.SectionCount, "length", r.Length)
	case wiki.MovePageResult:
		return append(attrs, "success", r.Success, "from", r.From, "to", r.To)
	case wiki.ManageCategoriesResult:
		return append(attrs, "added", len(r.Added), "removed", len(r.Removed))
	case wiki.GetStalePagesResult:
		return append(attrs, "stale_count", r.StaleCount, "scanned", r.TotalScanned)
	}
	return attrs
}
