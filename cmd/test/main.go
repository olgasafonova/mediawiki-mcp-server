// +build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/olgasafonova/nordic-registry-mcp-server/internal/norway"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := norway.NewClient(norway.DefaultConfig(), logger)

	ctx := context.Background()

	// Test with Equinor
	fmt.Println("=== Getting Equinor (923609016) ===")
	company, err := client.GetCompany(ctx, "923609016")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	jsonBytes, _ := json.MarshalIndent(company, "", "  ")
	fmt.Println(string(jsonBytes))

	// Test board members
	fmt.Println("\n=== Board Members ===")
	members, err := client.GetBoardMembers(ctx, "923609016")
	if err != nil {
		fmt.Printf("Error getting board: %v\n", err)
	} else {
		for _, m := range members {
			if m.Person != nil {
				fmt.Printf("- %s: %s\n", m.Position, m.Person.Name)
			}
		}
	}
}
