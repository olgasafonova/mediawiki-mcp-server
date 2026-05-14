# MediaWiki MCP Server Evaluations

This directory contains evaluation test suites for validating LLM tool selection accuracy with the MediaWiki MCP server.

## Overview

MCP (Model Context Protocol) evaluations test whether LLMs correctly:
1. **Select the right tool** for a given natural language request
2. **Disambiguate similar tools** that could easily be confused
3. **Extract correct arguments** from natural language inputs

## Test Suites

### `tool_selection.json`
Tests for correct tool selection across all tool categories:
- Search (wiki-wide vs page-specific)
- Read (pages, sections, info)
- History (revisions, contributions, recent changes)
- Categories and links
- Quality checks (terminology, translations, audits)
- Write operations (edit, find-replace, formatting)

### `confusion_pairs.json`
Tests for disambiguating commonly confused tool pairs:
- `mediawiki_search` vs `mediawiki_search_in_page`
- `mediawiki_get_page` vs `mediawiki_get_sections`
- `mediawiki_edit_page` vs `mediawiki_find_replace`
- `mediawiki_find_replace` vs `mediawiki_bulk_replace`
- `mediawiki_apply_formatting` vs `mediawiki_find_replace`
- And more...

### `argument_correctness.json`
Tests for correct argument extraction:
- Required arguments are present
- Expected values match
- Forbidden arguments are not used
- Types are correct (arrays, booleans, etc.)

## Running Evals

### Inspect the test data

```bash
# Summary of all suites
go run ./cmd/evals -dir ./evals -suite all

# One suite with full case detail
go run ./cmd/evals -dir ./evals -suite tool_selection -verbose
```

### Run evals against Claude (reference adapter)

The repo ships a Claude-backed `ToolSelector` so anyone with an Anthropic API key can run the evals end-to-end.

```bash
export ANTHROPIC_API_KEY=sk-ant-...

# Run all 112 tests against the default model (claude-sonnet-4-6)
go run ./cmd/evals -run

# Pick a different model or a single suite
go run ./cmd/evals -run -model claude-opus-4-7 -suite confusion_pairs

# Emit machine-readable results for CI
go run ./cmd/evals -run -json > results.json
```

Exit code is non-zero when any suite has failures, so CI can branch on it.

The tool definitions are prompt-cached across calls, so a full 112-test pass typically uses ~95% cached input tokens.

### Run the framework unit tests

```bash
go test ./evals/...
```

These exercise the loader and `EvaluateToolSelection` / `EvaluateConfusionPairs` / `EvaluateArguments` against mock selectors. They do not call any LLM.

## Bring Your Own Selector

The Claude adapter (`claude_selector.go`) is one implementation of the `ToolSelector` interface. To evaluate a different LLM, implement:

```go
type ToolSelector interface {
    SelectTool(input string) (toolName string, args map[string]interface{}, err error)
}
```

Then feed it to the same evaluation functions:

```go
import "github.com/olgasafonova/mediawiki-mcp-server/evals"

toolSelection, confusionPairs, arguments, _ := evals.LoadAllEvals("./evals")

selector := &MyLLMSelector{...}

metrics1, _ := evals.EvaluateToolSelection(toolSelection, selector)
metrics2, _ := evals.EvaluateConfusionPairs(confusionPairs, selector)
metrics3, _ := evals.EvaluateArguments(arguments, selector)

fmt.Println(evals.FormatMetrics(metrics1, "Tool Selection"))
fmt.Println(evals.FormatMetrics(metrics2, "Confusion Pairs"))
fmt.Println(evals.FormatMetrics(metrics3, "Argument Correctness"))
```

## Metrics

The evaluation framework tracks:

- **Accuracy**: Percentage of tests passed
- **By Category**: Breakdown by tool category
- **By Tool**: Per-tool precision and recall
- **False Positives**: Times wrong tool was selected
- **False Negatives**: Times correct tool was missed

## Best Practices for Tool Descriptions

Based on evaluation research, effective tool descriptions include:

1. **USE WHEN** section: Natural language triggers
2. **NOT FOR** section: Disambiguation from similar tools
3. **PARAMETERS**: With types and defaults
4. **RETURNS**: What the tool outputs
5. **EXAMPLES**: Real usage patterns

Example:
```
USE WHEN: User asks "find pages about X", "where is X documented"
NOT FOR: Searching within a specific known page (use mediawiki_search_in_page)
PARAMETERS:
- query: Search text (required)
- limit: Max results (default 10)
RETURNS: Page titles, snippets with highlights
```

## Adding New Tests

Follow the existing JSON schema when adding tests:

```json
{
  "id": "unique-test-id",
  "category": "search|read|write|etc",
  "input": "natural language user request",
  "expected_tool": "mediawiki_tool_name",
  "expected_args": {"arg": "value"},
  "not_tools": ["tools_that_should_not_be_selected"]
}
```

## References

- [MCP Evals Best Practices](https://mcpevals.io)
- [GitHub's MCP Server Evaluation Approach](https://github.blog/engineering/building-github-mcp-server/)
- [Neon Case Study on Tool Selection](https://neon.tech/blog/mcp-tool-selection)
