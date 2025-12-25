//go:build integration

package finland

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestIntegration_GetCompanyByID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test with Nokia Oyj
	company, err := client.GetCompanyByID(ctx, "0112038-9")
	if err != nil {
		t.Fatalf("GetCompanyByID failed: %v", err)
	}

	if company.BusinessID != "0112038-9" {
		t.Errorf("Expected 0112038-9, got %s", company.BusinessID)
	}

	// Check that we got a name
	hasName := false
	for _, name := range company.Names {
		if name.Name == "Nokia Oyj" {
			hasName = true
			break
		}
	}
	if !hasName {
		t.Error("Expected Nokia Oyj in names")
	}
}

func TestIntegration_Search(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.Search(ctx, SearchParams{
		Name:       "Nokia",
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Results) == 0 {
		t.Error("Expected at least one result")
	}
}

func TestIntegration_GetCompany(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	company, err := client.GetCompany(ctx, "0112038-9")
	if err != nil {
		t.Fatalf("GetCompany failed: %v", err)
	}

	if company.Name != "Nokia Oyj" {
		t.Errorf("Expected Nokia Oyj, got %s", company.Name)
	}

	if company.Status != "active" {
		t.Errorf("Expected active status, got %s", company.Status)
	}

	if company.Country != "FI" {
		t.Errorf("Expected FI, got %s", company.Country)
	}
}

func TestIntegration_SearchCompanies(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.SearchCompanies(ctx, "Nokia", 5)
	if err != nil {
		t.Fatalf("SearchCompanies failed: %v", err)
	}

	if len(result.Companies) == 0 {
		t.Error("Expected at least one result")
	}

	// Check for Nokia Oyj
	found := false
	for _, c := range result.Companies {
		if c.OrgNumber == "0112038-9" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected Nokia Oyj (0112038-9) in results")
	}
}
