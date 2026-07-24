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

// evalOptions carries the parsed CLI flags shared by all subcommands.
type evalOptions struct {
	dir     string
	suite   string
	model   string
	verbose bool
	jsonOut bool
}

func main() {
	dir := flag.String("dir", "./evals", "Directory containing eval JSON files")
	suite := flag.String("suite", "all", "Suite: tool_selection, confusion_pairs, arguments, or all")
	verbose := flag.Bool("verbose", false, "Show detailed test information (inspect mode only)")
	run := flag.Bool("run", false, "Run the evals against Claude (requires ANTHROPIC_API_KEY)")
	model := flag.String("model", "", "Claude model (default: claude-sonnet-4-6)")
	jsonOut := flag.Bool("json", false, "Emit results as JSON (run mode only)")
	flag.Parse()

	opts := evalOptions{
		dir:     *dir,
		suite:   *suite,
		model:   *model,
		verbose: *verbose,
		jsonOut: *jsonOut,
	}

	if *run {
		runEvals(opts)
		return
	}

	fmt.Println("MediaWiki MCP Server - Evaluation Framework")
	fmt.Println("============================================")
	fmt.Println()

	switch opts.suite {
	case "tool_selection":
		loadToolSelection(opts)
	case "confusion_pairs":
		loadConfusionPairs(opts)
	case "arguments":
		loadArguments(opts)
	case "all":
		loadAll(opts)
	default:
		fmt.Fprintf(os.Stderr, "Unknown suite: %s\n", opts.suite)
		os.Exit(1)
	}
}

// suiteSpec describes one runnable eval suite: its CLI key, display label,
// and a loader+evaluator closure.
type suiteSpec struct {
	key   string
	label string
	run   func(opts evalOptions, selector *evals.ClaudeSelector) (*evals.EvalMetrics, error)
}

var suiteSpecs = []suiteSpec{
	{"tool_selection", "Tool Selection", func(opts evalOptions, selector *evals.ClaudeSelector) (*evals.EvalMetrics, error) {
		s, err := evals.LoadToolSelectionSuite(filepath.Join(opts.dir, "tool_selection.json"))
		if err != nil {
			return nil, err
		}
		metrics, _ := evals.EvaluateToolSelection(s, selector)
		return metrics, nil
	}},
	{"confusion_pairs", "Confusion Pairs", func(opts evalOptions, selector *evals.ClaudeSelector) (*evals.EvalMetrics, error) {
		s, err := evals.LoadConfusionPairSuite(filepath.Join(opts.dir, "confusion_pairs.json"))
		if err != nil {
			return nil, err
		}
		metrics, _ := evals.EvaluateConfusionPairs(s, selector)
		return metrics, nil
	}},
	{"arguments", "Argument Correctness", func(opts evalOptions, selector *evals.ClaudeSelector) (*evals.EvalMetrics, error) {
		s, err := evals.LoadArgumentSuite(filepath.Join(opts.dir, "argument_correctness.json"))
		if err != nil {
			return nil, err
		}
		metrics, _ := evals.EvaluateArguments(s, selector)
		return metrics, nil
	}},
}

func runEvals(opts evalOptions) {
	selector, err := evals.NewClaudeSelector(opts.model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing Claude selector: %v\n", err)
		os.Exit(1)
	}

	printRunHeader(opts)
	results := runSelectedSuites(opts, selector)

	if opts.jsonOut {
		emitJSONResults(results)
	}
	exitOnFailures(results)
}

// runSelectedSuites runs every suite matching opts.suite and returns
// the metrics keyed by suite name. Load errors are fatal.
func runSelectedSuites(opts evalOptions, selector *evals.ClaudeSelector) map[string]*evals.EvalMetrics {
	results := make(map[string]*evals.EvalMetrics)
	for _, spec := range suiteSpecs {
		if opts.suite != spec.key && opts.suite != "all" {
			continue
		}
		metrics, err := spec.run(opts, selector)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", spec.key, err)
			os.Exit(1)
		}
		results[spec.key] = metrics
		if !opts.jsonOut {
			fmt.Print(evals.FormatMetrics(metrics, spec.label))
		}
	}
	return results
}

// printRunHeader prints the run banner unless JSON output is requested.
func printRunHeader(opts evalOptions) {
	if opts.jsonOut {
		return
	}
	modelName := opts.model
	if modelName == "" {
		modelName = "claude-sonnet-4-6 (default)"
	}
	fmt.Printf("Running evals against %s\n", modelName)
	fmt.Println("Suite:", opts.suite)
	fmt.Println()
}

// emitJSONResults writes the metrics map as indented JSON to stdout.
func emitJSONResults(results map[string]*evals.EvalMetrics) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(results); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding results: %v\n", err)
		os.Exit(1)
	}
}

// exitOnFailures exits non-zero when any suite has failures, so CI can branch on it.
func exitOnFailures(results map[string]*evals.EvalMetrics) {
	for _, m := range results {
		if m.FailedTests > 0 {
			os.Exit(1)
		}
	}
}

func loadToolSelection(opts evalOptions) {
	path := filepath.Join(opts.dir, "tool_selection.json")
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

	if opts.verbose {
		printToolSelectionTests(suite)
	}
}

// printToolSelectionTests lists every tool-selection test case.
func printToolSelectionTests(suite *evals.ToolSelectionSuite) {
	fmt.Println("Test Cases:")
	for _, test := range suite.Tests {
		fmt.Printf("  [%s] %s\n", test.ID, test.Input)
		fmt.Printf("    -> %s\n", test.ExpectedTool)
		if len(test.NotTools) > 0 {
			fmt.Printf("    not: %v\n", test.NotTools)
		}
	}
}

func loadConfusionPairs(opts evalOptions) {
	path := filepath.Join(opts.dir, "confusion_pairs.json")
	suite, err := evals.LoadConfusionPairSuite(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading confusion pairs suite: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Confusion Pairs Suite: %s\n", suite.Name)
	fmt.Printf("Version: %s\n", suite.Version)
	fmt.Printf("Description: %s\n", suite.Description)
	fmt.Printf("Total Pairs: %d\n", len(suite.Pairs))
	fmt.Printf("Total Tests: %d\n", countConfusionTests(suite))
	fmt.Println()

	fmt.Println("Confusion Pairs:")
	for _, pair := range suite.Pairs {
		fmt.Printf("\n  %s:\n", pair.ID)
		fmt.Printf("    Tools: %v\n", pair.Tools)
		fmt.Printf("    Rule: %s\n", pair.Disambiguation)
		fmt.Printf("    Tests: %d\n", len(pair.Tests))

		if opts.verbose {
			for _, test := range pair.Tests {
				fmt.Printf("      \"%s\"\n", test.Input)
				fmt.Printf("        -> %s (%s)\n", test.Expected, test.Reason)
			}
		}
	}
	fmt.Println()
}

func loadArguments(opts evalOptions) {
	path := filepath.Join(opts.dir, "argument_correctness.json")
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

	if opts.verbose {
		printArgumentTests(suite)
	}
}

// printArgumentTests lists every argument-correctness test case.
func printArgumentTests(suite *evals.ArgumentSuite) {
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

func loadAll(opts evalOptions) {
	toolSelection, confusionPairs, arguments, err := evals.LoadAllEvals(opts.dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading evals: %v\n", err)
		os.Exit(1)
	}

	confusionTests := countConfusionTests(confusionPairs)
	totalTests := len(toolSelection.Tests) + confusionTests + len(arguments.Tests)

	fmt.Printf("Loaded all evaluation suites from: %s\n\n", opts.dir)

	fmt.Println("Summary:")
	fmt.Println("--------")
	fmt.Printf("Tool Selection Tests:   %d\n", len(toolSelection.Tests))
	fmt.Printf("Confusion Pair Tests:   %d (across %d pairs)\n", confusionTests, len(confusionPairs.Pairs))
	fmt.Printf("Argument Tests:         %d\n", len(arguments.Tests))
	fmt.Printf("----------------------\n")
	fmt.Printf("Total Evaluation Tests: %d\n", totalTests)
	fmt.Println()

	toolCoverage := collectToolCoverage(toolSelection, confusionPairs, arguments)
	fmt.Printf("Tool Coverage: %d unique tools tested\n", len(toolCoverage))

	if opts.verbose {
		fmt.Println("\nCovered Tools:")
		for tool := range toolCoverage {
			fmt.Printf("  - %s\n", tool)
		}
	}

	fmt.Println()
	fmt.Println("Run against Claude: go run ./cmd/evals -run  (requires ANTHROPIC_API_KEY)")
}

// countConfusionTests sums the test cases across all confusion pairs.
func countConfusionTests(confusionPairs *evals.ConfusionPairSuite) int {
	total := 0
	for _, pair := range confusionPairs.Pairs {
		total += len(pair.Tests)
	}
	return total
}

// collectToolCoverage gathers the set of unique tools referenced by any suite.
func collectToolCoverage(toolSelection *evals.ToolSelectionSuite, confusionPairs *evals.ConfusionPairSuite, arguments *evals.ArgumentSuite) map[string]bool {
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
	return toolCoverage
}
