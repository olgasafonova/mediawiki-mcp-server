//go:build integration

package wiki

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

// Integration tests for MediaWiki MCP Server
//
// These tests require a running MediaWiki instance.
// Start the test environment with:
//   docker-compose -f docker-compose.test.yml up -d
//
// Run integration tests with:
//   go test -tags=integration ./wiki/...
//
// Environment variables:
//   MEDIAWIKI_URL      - MediaWiki API URL (default: http://localhost:8080/api.php)
//   MEDIAWIKI_USERNAME - Optional: username for authenticated tests
//   MEDIAWIKI_PASSWORD - Optional: password for authenticated tests

func getIntegrationConfig(t *testing.T) *Config {
	t.Helper()

	url := os.Getenv("MEDIAWIKI_URL")
	if url == "" {
		url = "http://localhost:8080/api.php"
	}

	return &Config{
		BaseURL:    url,
		Username:   os.Getenv("MEDIAWIKI_USERNAME"),
		Password:   os.Getenv("MEDIAWIKI_PASSWORD"),
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		UserAgent:  "MediaWiki-MCP-Server-IntegrationTest/1.0",
	}
}

func createIntegrationClient(t *testing.T) *Client {
	t.Helper()
	config := getIntegrationConfig(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return NewClient(config, logger)
}

func TestIntegration_GetSiteInfo(t *testing.T) {
	client := createIntegrationClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := client.GetSiteInfo(ctx)
	if err != nil {
		t.Fatalf("GetSiteInfo failed: %v", err)
	}

	if info.SiteName == "" {
		t.Error("Expected non-empty site name")
	}

	if info.ArticlePath == "" {
		t.Error("Expected non-empty article path")
	}

	t.Logf("Connected to wiki: %s (generator: %s)", info.SiteName, info.Generator)
}

func TestIntegration_Search(t *testing.T) {
	client := createIntegrationClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Search for main page (should exist on any wiki)
	results, err := client.Search(ctx, SearchArgs{
		Query: "Main",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	t.Logf("Search returned %d results", len(results.Results))
}

func TestIntegration_GetPage(t *testing.T) {
	client := createIntegrationClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to get the main page
	page, err := client.GetPage(ctx, GetPageArgs{
		Title: "Main Page",
	})
	if err != nil {
		// Main Page might not exist, that's OK for this test
		t.Logf("GetPage returned error (may be expected): %v", err)
		return
	}

	if page.Title == "" {
		t.Error("Expected non-empty page title")
	}

	t.Logf("Got page: %s (length: %d)", page.Title, len(page.Content))
}

func TestIntegration_ListCategories(t *testing.T) {
	client := createIntegrationClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	categories, err := client.ListCategories(ctx, ListCategoriesArgs{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}

	t.Logf("Listed %d categories", len(categories.Categories))
}

func TestIntegration_GetRecentChanges(t *testing.T) {
	client := createIntegrationClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	changes, err := client.GetRecentChanges(ctx, GetRecentChangesArgs{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("GetRecentChanges failed: %v", err)
	}

	t.Logf("Got %d recent changes", len(changes.Changes))
}

func TestIntegration_AuthenticatedOperations(t *testing.T) {
	config := getIntegrationConfig(t)

	// Skip if no credentials provided
	if config.Username == "" || config.Password == "" {
		t.Skip("Skipping authenticated tests: MEDIAWIKI_USERNAME and MEDIAWIKI_PASSWORD not set")
	}

	client := createIntegrationClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Test login
	err := client.Login(ctx)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	t.Log("Login successful")

	// Test creating a page
	testTitle := "Integration_Test_Page_" + time.Now().Format("20060102_150405")
	testContent := "This is a test page created by integration tests.\n\n[[Category:Test]]"

	result, err := client.CreatePage(ctx, CreatePageArgs{
		Title:   testTitle,
		Content: testContent,
		Summary: "Integration test: creating test page",
	})
	if err != nil {
		t.Fatalf("CreatePage failed: %v", err)
	}
	t.Logf("Created page: %s (revision: %d)", result.Title, result.RevisionID)

	// Test reading the created page
	page, err := client.GetPage(ctx, GetPageArgs{
		Title: testTitle,
	})
	if err != nil {
		t.Fatalf("GetPage failed after creation: %v", err)
	}

	if page.Title != testTitle {
		t.Errorf("Expected title %q, got %q", testTitle, page.Title)
	}

	// Test editing the page
	updatedContent := testContent + "\n\nUpdated by integration test."
	editResult, err := client.EditPage(ctx, EditPageArgs{
		Title:   testTitle,
		Content: updatedContent,
		Summary: "Integration test: updating test page",
	})
	if err != nil {
		t.Fatalf("EditPage failed: %v", err)
	}
	t.Logf("Edited page: %s (new revision: %d)", editResult.Title, editResult.RevisionID)

	// Test getting page history
	history, err := client.GetPageHistory(ctx, GetPageHistoryArgs{
		Title: testTitle,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("GetPageHistory failed: %v", err)
	}

	if len(history.Revisions) < 2 {
		t.Errorf("Expected at least 2 revisions, got %d", len(history.Revisions))
	}
	t.Logf("Page has %d revisions", len(history.Revisions))

	// Cleanup: delete the test page
	deleteResult, err := client.DeletePage(ctx, DeletePageArgs{
		Title:  testTitle,
		Reason: "Integration test cleanup",
	})
	if err != nil {
		t.Logf("Warning: DeletePage failed (may need sysop rights): %v", err)
	} else {
		t.Logf("Deleted page: %s", deleteResult.Title)
	}
}
