//go:build integration

package denmark

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestIntegration_GetCompanyByCVR(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test with Novo Nordisk A/S
	company, err := client.GetCompanyByCVR(ctx, "24256790")
	if err != nil {
		t.Fatalf("GetCompanyByCVR failed: %v", err)
	}

	if company.Name != "NOVO NORDISK A/S" {
		t.Errorf("Expected NOVO NORDISK A/S, got %s", company.Name)
	}

	if company.VAT != 24256790 {
		t.Errorf("Expected 24256790, got %d", company.VAT)
	}
}

func TestIntegration_Search(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	company, err := client.Search(ctx, "Novo Nordisk")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if company == nil {
		t.Fatal("Expected company result")
	}

	// Should return Novo Nordisk
	if company.VAT != 24256790 {
		t.Logf("Note: Search returned %s (CVR %d) instead of Novo Nordisk A/S", company.Name, company.VAT)
	}
}

func TestIntegration_GetCompany(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	company, err := client.GetCompany(ctx, "24256790")
	if err != nil {
		t.Fatalf("GetCompany failed: %v", err)
	}

	if company.Name != "NOVO NORDISK A/S" {
		t.Errorf("Expected NOVO NORDISK A/S, got %s", company.Name)
	}

	if company.Status != "active" {
		t.Errorf("Expected active status, got %s", company.Status)
	}

	if company.Country != "DK" {
		t.Errorf("Expected DK, got %s", company.Country)
	}

	if company.Employees == 0 {
		t.Error("Expected employee count > 0")
	}
}

func TestIntegration_SearchCompanies(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.SearchCompanies(ctx, "LEGO", 5)
	if err != nil {
		t.Fatalf("SearchCompanies failed: %v", err)
	}

	if len(result.Companies) == 0 {
		t.Error("Expected at least one result")
	}
}
