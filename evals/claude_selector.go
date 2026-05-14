// Package evals — Claude reference adapter.
//
// ClaudeSelector implements ToolSelector by asking Claude to pick a tool for
// each natural-language test input. It's the reference implementation for
// running the eval suites against a real LLM; users can model their own
// selectors on it.
//
// Requires ANTHROPIC_API_KEY in the environment.
package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/olgasafonova/mediawiki-mcp-server/tools"
)

const claudeSelectorSystem = `You evaluate tool selection for a MediaWiki MCP server. For each user request, call exactly one tool that best matches the intent. Use the USE WHEN and NOT FOR sections in tool descriptions to disambiguate similar tools. Extract argument values from the user request as faithfully as possible.`

// ClaudeSelector asks Claude which tool to call for a given input.
type ClaudeSelector struct {
	client   anthropic.Client
	model    anthropic.Model
	toolDefs []anthropic.ToolUnionParam
	timeout  time.Duration
}

// NewClaudeSelector builds a selector backed by the Anthropic API.
// An empty model string defaults to claude-sonnet-4-6.
func NewClaudeSelector(model string) (*ClaudeSelector, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
	}
	if model == "" {
		model = anthropic.ModelClaudeSonnet4_6
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	toolDefs := make([]anthropic.ToolUnionParam, 0, len(tools.AllTools))
	for _, spec := range tools.AllTools {
		toolDefs = append(toolDefs, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        spec.Name,
				Description: anthropic.String(spec.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{},
				},
			},
		})
	}
	// Cache the tool block — every test reuses the same definitions.
	// The breakpoint on the last tool caches all preceding tool defs.
	if len(toolDefs) > 0 {
		toolDefs[len(toolDefs)-1].OfTool.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	return &ClaudeSelector{
		client:   client,
		model:    model,
		toolDefs: toolDefs,
		timeout:  30 * time.Second,
	}, nil
}

// SelectTool calls Claude once and returns the first tool_use block.
func (c *ClaudeSelector) SelectTool(input string) (string, map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 1024,
		Tools:     c.toolDefs,
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{},
		},
		System: []anthropic.TextBlockParam{{Text: claudeSelectorSystem}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(input)),
		},
	})
	if err != nil {
		return "", nil, fmt.Errorf("claude API: %w", err)
	}

	for _, block := range resp.Content {
		if block.Type != "tool_use" {
			continue
		}
		tu := block.AsToolUse()
		var args map[string]any
		if len(tu.Input) > 0 {
			if err := json.Unmarshal(tu.Input, &args); err != nil {
				return tu.Name, nil, fmt.Errorf("parsing tool input: %w", err)
			}
		}
		return tu.Name, args, nil
	}

	return "", nil, fmt.Errorf("no tool_use block in response")
}
