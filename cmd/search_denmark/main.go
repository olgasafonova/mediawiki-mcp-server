// +build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/olgasafonova/nordic-registry-mcp-server/internal/denmark"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := denmark.NewClient(denmark.DefaultConfig(), logger)

	ctx := context.Background()
	result, err := client.SearchCompanies(ctx, "Novo Nordisk", 10)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(jsonBytes))
}
