// Command evals inspects or runs the MediaWiki MCP tool selection eval suites.
//
// Inspect (default):
//
//	go run ./cmd/evals -dir ./evals -suite all
//
// Run against Claude (requires ANTHROPIC_API_KEY):
//
//	go run ./cmd/evals -run
//	go run ./cmd/evals -run -model claude-opus-4-7 -suite confusion_pairs
//	go run ./cmd/evals -run -json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/olgasafonova/mediawiki-mcp-server/evals"
)

func main() {
	dir := flag.String("dir", "./evals", "Directory containing eval JSON files")
	suite := flag.String("suite", "all", "Suite: tool_selection, confusion_pairs, arguments, or all")
	verbose := flag.Bool("verbose", false, "Show detailed test information (inspect mode only)")
	run := flag.Bool("run", false, "Run the evals against Claude (requires ANTHROPIC_API_KEY)")
	model := flag.String("model", "", "Claude model (default: claude-sonnet-4-6)")
	jsonOut := flag.Bool("json", false, "Emit results as JSON (run mode only)")
	flag.Parse()

	if *run {
		runEvals(*dir, *suite, *model, *jsonOut)
		return
	}

	fmt.Println("MediaWiki MCP Server - Evaluation Framework")
	fmt.Println("============================================")
	fmt.Println()

	switch *suite {
	case "tool_selection":
		loadToolSelection(*dir, *verbose)
	case "confusion_pairs":
		loadConfusionPairs(*dir, *verbose)
	case "arguments":
		loadArguments(*dir, *verbose)
	case "all":
		loadAll(*dir, *verbose)
	default:
		fmt.Fprintf(os.Stderr, "Unknown suite: %s\n", *suite)
		os.Exit(1)
	}
}

func runEvals(dir, suite, model string, jsonOut bool) {
	selector, err := evals.NewClaudeSelector(model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing Claude selector: %v\n", err)
		os.Exit(1)
	}

	if !jsonOut {
		modelName := model
		if modelName == "" {
			modelName = "claude-sonnet-4-6 (default)"
		}
		fmt.Printf("Running evals against %s\n", modelName)
		fmt.Println("Suite:", suite)
		fmt.Println()
	}

	results := make(map[string]*evals.EvalMetrics)

	if suite == "tool_selection" || suite == "all" {
		s, err := evals.LoadToolSelectionSuite(filepath.Join(dir, "tool_selection.json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading tool_selection: %v\n", err)
			os.Exit(1)
		}
		metrics, _ := evals.EvaluateToolSelection(s, selector)
		results["tool_selection"] = metrics
		if !jsonOut {
			fmt.Print(evals.FormatMetrics(metrics, "Tool Selection"))
		}
	}

	if suite == "confusion_pairs" || suite == "all" {
		s, err := evals.LoadConfusionPairSuite(filepath.Join(dir, "confusion_pairs.json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading confusion_pairs: %v\n", err)
			os.Exit(1)
		}
		metrics, _ := evals.EvaluateConfusionPairs(s, selector)
		results["confusion_pairs"] = metrics
		if !jsonOut {
			fmt.Print(evals.FormatMetrics(metrics, "Confusion Pairs"))
		}
	}

	if suite == "arguments" || suite == "all" {
		s, err := evals.LoadArgumentSuite(filepath.Join(dir, "argument_correctness.json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading arguments: %v\n", err)
			os.Exit(1)
		}
		metrics, _ := evals.EvaluateArguments(s, selector)
		results["arguments"] = metrics
		if !jsonOut {
			fmt.Print(evals.FormatMetrics(metrics, "Argument Correctness"))
		}
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding results: %v\n", err)
			os.Exit(1)
		}
	}

	// Non-zero exit when any suite has failures, so CI can branch on it.
	for _, m := range results {
		if m.FailedTests > 0 {
			os.Exit(1)
		}
	}
}

func loadToolSelection(dir string, verbose bool) {
	path := filepath.Join(dir, "tool_selection.json")
	suite, err := evals.LoadToolSelectionSuite(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading tool selection suite: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Tool Selection Suite: %s\n", suite.Name)
	fmt.Printf("Version: %s\n", suite.Version)
	fmt.Printf("Description: %s\n", suite.Description)
	fmt.Printf("Total Tests: %d\n", len(suite.Tests))
	fmt.Println()

	// Count by category
	categories := make(map[string]int)
	tools := make(map[string]int)
	for _, test := range suite.Tests {
		categories[test.Category]++
		tools[test.ExpectedTool]++
	}

	fmt.Println("Tests by Category:")
	for cat, count := range categories {
		fmt.Printf("  %-15s: %d\n", cat, count)
	}
	fmt.Println()

	fmt.Println("Tests by Tool:")
	for tool, count := range tools {
		fmt.Printf("  %-40s: %d\n", tool, count)
	}
	fmt.Println()

	if verbose {
		fmt.Println("Test Cases:")
		for _, test := range suite.Tests {
			fmt.Printf("  [%s] %s\n", test.ID, test.Input)
			fmt.Printf("    -> %s\n", test.ExpectedTool)
			if len(test.NotTools) > 0 {
				fmt.Printf("    not: %v\n", test.NotTools)
			}
		}
	}
}

func loadConfusionPairs(dir string, verbose bool) {
	path := filepath.Join(dir, "confusion_pairs.json")
	suite, err := evals.LoadConfusionPairSuite(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading confusion pairs suite: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Confusion Pairs Suite: %s\n", suite.Name)
	fmt.Printf("Version: %s\n", suite.Version)
	fmt.Printf("Description: %s\n", suite.Description)
	fmt.Printf("Total Pairs: %d\n", len(suite.Pairs))

	totalTests := 0
	for _, pair := range suite.Pairs {
		totalTests += len(pair.Tests)
	}
	fmt.Printf("Total Tests: %d\n", totalTests)
	fmt.Println()

	fmt.Println("Confusion Pairs:")
	for _, pair := range suite.Pairs {
		fmt.Printf("\n  %s:\n", pair.ID)
		fmt.Printf("    Tools: %v\n", pair.Tools)
		fmt.Printf("    Rule: %s\n", pair.Disambiguation)
		fmt.Printf("    Tests: %d\n", len(pair.Tests))

		if verbose {
			for _, test := range pair.Tests {
				fmt.Printf("      \"%s\"\n", test.Input)
				fmt.Printf("        -> %s (%s)\n", test.Expected, test.Reason)
			}
		}
	}
	fmt.Println()
}

func loadArguments(dir string, verbose bool) {
	path := filepath.Join(dir, "argument_correctness.json")
	suite, err := evals.LoadArgumentSuite(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading argument suite: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Argument Suite: %s\n", suite.Name)
	fmt.Printf("Version: %s\n", suite.Version)
	fmt.Printf("Description: %s\n", suite.Description)
	fmt.Printf("Total Tests: %d\n", len(suite.Tests))
	fmt.Println()

	// Count by tool
	tools := make(map[string]int)
	for _, test := range suite.Tests {
		tools[test.Tool]++
	}

	fmt.Println("Tests by Tool:")
	for tool, count := range tools {
		fmt.Printf("  %-40s: %d\n", tool, count)
	}
	fmt.Println()

	fmt.Println("Validation Rules:")
	fmt.Printf("  Title Format: %s\n", suite.ValidationRules.TitleFormat)
	fmt.Printf("  Category Format: %s\n", suite.ValidationRules.CategoryFormat)
	fmt.Printf("  Boolean Handling: %s\n", suite.ValidationRules.BooleanHandling)
	fmt.Printf("  Array Handling: %s\n", suite.ValidationRules.ArrayHandling)
	fmt.Printf("  Preview Default: %s\n", suite.ValidationRules.PreviewDefault)
	fmt.Println()

	if verbose {
		fmt.Println("Test Cases:")
		for _, test := range suite.Tests {
			fmt.Printf("  [%s] %s\n", test.ID, test.Input)
			fmt.Printf("    Tool: %s\n", test.Tool)
			fmt.Printf("    Required: %v\n", test.RequiredArgs)
			fmt.Printf("    Expected: %v\n", test.ExpectedArgs)
			if len(test.ForbiddenArgs) > 0 {
				fmt.Printf("    Forbidden: %v\n", test.ForbiddenArgs)
			}
			if test.ArgNotes != "" {
				fmt.Printf("    Notes: %s\n", test.ArgNotes)
			}
		}
	}
}

func loadAll(dir string, verbose bool) {
	toolSelection, confusionPairs, arguments, err := evals.LoadAllEvals(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading evals: %v\n", err)
		os.Exit(1)
	}

	// Count totals
	totalTests := len(toolSelection.Tests)
	for _, pair := range confusionPairs.Pairs {
		totalTests += len(pair.Tests)
	}
	totalTests += len(arguments.Tests)

	fmt.Printf("Loaded all evaluation suites from: %s\n\n", dir)

	fmt.Println("Summary:")
	fmt.Println("--------")
	fmt.Printf("Tool Selection Tests:   %d\n", len(toolSelection.Tests))

	confusionTests := 0
	for _, pair := range confusionPairs.Pairs {
		confusionTests += len(pair.Tests)
	}
	fmt.Printf("Confusion Pair Tests:   %d (across %d pairs)\n", confusionTests, len(confusionPairs.Pairs))
	fmt.Printf("Argument Tests:         %d\n", len(arguments.Tests))
	fmt.Printf("----------------------\n")
	fmt.Printf("Total Evaluation Tests: %d\n", totalTests)
	fmt.Println()

	// Show tool coverage
	toolCoverage := make(map[string]bool)
	for _, test := range toolSelection.Tests {
		toolCoverage[test.ExpectedTool] = true
	}
	for _, pair := range confusionPairs.Pairs {
		for _, tool := range pair.Tools {
			toolCoverage[tool] = true
		}
	}
	for _, test := range arguments.Tests {
		toolCoverage[test.Tool] = true
	}

	fmt.Printf("Tool Coverage: %d unique tools tested\n", len(toolCoverage))

	if verbose {
		fmt.Println("\nCovered Tools:")
		for tool := range toolCoverage {
			fmt.Printf("  - %s\n", tool)
		}
	}

	fmt.Println()
	fmt.Println("Run against Claude: go run ./cmd/evals -run  (requires ANTHROPIC_API_KEY)")
}
