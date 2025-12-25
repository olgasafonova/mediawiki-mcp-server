//go:build integration

package norway

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestIntegration_GetEnhet(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test with Equinor ASA
	enhet, err := client.GetEnhet(ctx, "923609016")
	if err != nil {
		t.Fatalf("GetEnhet failed: %v", err)
	}

	if enhet.Navn != "EQUINOR ASA" {
		t.Errorf("Expected EQUINOR ASA, got %s", enhet.Navn)
	}

	if enhet.Organisasjonsnummer != "923609016" {
		t.Errorf("Expected 923609016, got %s", enhet.Organisasjonsnummer)
	}

	if enhet.Organisasjonsform.Kode != "ASA" {
		t.Errorf("Expected ASA, got %s", enhet.Organisasjonsform.Kode)
	}
}

func TestIntegration_SearchEnheter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.SearchEnheter(ctx, SearchParams{
		Navn: "Equinor",
		Size: 5,
	})
	if err != nil {
		t.Fatalf("SearchEnheter failed: %v", err)
	}

	if len(result.Embedded.Enheter) == 0 {
		t.Error("Expected at least one result")
	}

	// Check that EQUINOR ASA is in the results
	found := false
	for _, e := range result.Embedded.Enheter {
		if e.Organisasjonsnummer == "923609016" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected EQUINOR ASA (923609016) in search results")
	}
}

func TestIntegration_GetRoller(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	roles, err := client.GetRoller(ctx, "923609016")
	if err != nil {
		t.Fatalf("GetRoller failed: %v", err)
	}

	if len(roles.Rollegrupper) == 0 {
		t.Error("Expected at least one role group")
	}
}

func TestIntegration_GetCompany(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	company, err := client.GetCompany(ctx, "923609016")
	if err != nil {
		t.Fatalf("GetCompany failed: %v", err)
	}

	if company.Name != "EQUINOR ASA" {
		t.Errorf("Expected EQUINOR ASA, got %s", company.Name)
	}

	if company.Status != "active" {
		t.Errorf("Expected active status, got %s", company.Status)
	}

	if company.Country != "NO" {
		t.Errorf("Expected NO, got %s", company.Country)
	}
}

func TestIntegration_GetBoardMembers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := NewClient(DefaultConfig(), logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	members, err := client.GetBoardMembers(ctx, "923609016")
	if err != nil {
		t.Fatalf("GetBoardMembers failed: %v", err)
	}

	if len(members) == 0 {
		t.Error("Expected at least one board member")
	}

	// Check that at least one member has a position
	hasPosition := false
	for _, m := range members {
		if m.Position != "" {
			hasPosition = true
			break
		}
	}
	if !hasPosition {
		t.Error("Expected at least one member with a position")
	}
}
