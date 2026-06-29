package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"runtime/debug"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

// registerResources adds MCP resources for direct wiki page access
func registerResources(server *mcp.Server, client *wiki.Client, logger *slog.Logger) {
	// Resource template for wiki pages
	// URI format: wiki://page/{title}
	// Example: wiki://page/Main_Page
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "wiki://page/{title}",
		Name:        "Wiki Page",
		Description: "Access MediaWiki page content directly. Use URL-encoded page titles (e.g., 'Main_Page' or 'Category%3AHelp').",
		MIMEType:    "text/plain",
	}, makePageResourceHandler(client, logger))

	// Resource template for wiki categories
	// URI format: wiki://category/{name}
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "wiki://category/{name}",
		Name:        "Wiki Category",
		Description: "List pages in a MediaWiki category. Use URL-encoded category names without 'Category:' prefix.",
		MIMEType:    "application/json",
	}, makeCategoryResourceHandler(client, logger))
}

// recoverResourceHandler logs a recovered panic from a resource handler.
func recoverResourceHandler(logger *slog.Logger, context string) {
	if r := recover(); r != nil {
		logger.Error("Panic recovered in "+context,
			"panic", r,
			"stack", string(debug.Stack()))
	}
}

// decodeResourceTitle validates the URI prefix and returns the decoded segment.
func decodeResourceTitle(uri, prefix, label string) (string, error) {
	if !strings.HasPrefix(uri, prefix) {
		return "", mcp.ResourceNotFoundError(uri)
	}
	decoded, err := url.PathUnescape(strings.TrimPrefix(uri, prefix))
	if err != nil {
		return "", fmt.Errorf("invalid %s encoding: %w", label, err)
	}
	if decoded == "" {
		return "", fmt.Errorf("%s cannot be empty", label)
	}
	return decoded, nil
}

// makePageResourceHandler returns the wiki://page/{title} resource handler.
func makePageResourceHandler(client *wiki.Client, logger *slog.Logger) func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		defer recoverResourceHandler(logger, "resource handler")

		uri := req.Params.URI
		title, err := decodeResourceTitle(uri, "wiki://page/", "page title")
		if err != nil {
			return nil, err
		}

		result, err := client.GetPage(ctx, wiki.GetPageArgs{
			Title:  title,
			Format: "wikitext",
		})
		if err != nil {
			logger.Warn("Failed to read wiki page resource",
				"uri", uri,
				"title", title,
				"error", err,
			)
			return nil, mcp.ResourceNotFoundError(uri)
		}

		logger.Info("Resource accessed",
			"uri", uri,
			"title", result.Title,
			"page_id", result.PageID,
		)

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      uri,
				MIMEType: "text/plain",
				Text:     result.Content,
			}},
		}, nil
	}
}

// makeCategoryResourceHandler returns the wiki://category/{name} resource handler.
func makeCategoryResourceHandler(client *wiki.Client, logger *slog.Logger) func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		defer recoverResourceHandler(logger, "category resource handler")

		uri := req.Params.URI
		name, err := decodeResourceTitle(uri, "wiki://category/", "category name")
		if err != nil {
			return nil, err
		}

		result, err := client.GetCategoryMembers(ctx, wiki.CategoryMembersArgs{
			Category: name,
			Limit:    100,
		})
		if err != nil {
			logger.Warn("Failed to read wiki category resource",
				"uri", uri,
				"category", name,
				"error", err,
			)
			return nil, mcp.ResourceNotFoundError(uri)
		}

		// Format as simple text list
		var content strings.Builder
		content.WriteString(fmt.Sprintf("Category: %s\n", name))
		content.WriteString(fmt.Sprintf("Pages: %d\n\n", len(result.Members)))
		for _, page := range result.Members {
			content.WriteString(fmt.Sprintf("- %s\n", page.Title))
		}
		if result.HasMore {
			content.WriteString("\n[More pages available...]")
		}

		logger.Info("Category resource accessed",
			"uri", uri,
			"category", name,
			"pages", len(result.Members),
		)

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      uri,
				MIMEType: "text/plain",
				Text:     content.String(),
			}},
		}, nil
	}
}
