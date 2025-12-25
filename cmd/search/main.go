// +build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/olgasafonova/nordic-registry-mcp-server/internal/finland"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := finland.NewClient(finland.DefaultConfig(), logger)

	ctx := context.Background()
	result, err := client.SearchCompanies(ctx, "Nokia Oyj", 10)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(jsonBytes))
}
