// Package evals provides evaluation framework for testing MCP tool selection accuracy.
// It validates that LLMs select the correct tools and extract proper arguments
// from natural language inputs.
package evals

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// loadSuite is the shared JSON-from-file decoder used by every eval suite
// loader. Centralizing it removes duplication and keeps error wrapping uniform.
func loadSuite[T any](path string) (*T, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is controlled by eval framework
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	var suite T
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return &suite, nil
}

// LoadToolSelectionSuite loads tool selection tests from a JSON file
func LoadToolSelectionSuite(path string) (*ToolSelectionSuite, error) {
	return loadSuite[ToolSelectionSuite](path)
}

// LoadConfusionPairSuite loads confusion pair tests from a JSON file
func LoadConfusionPairSuite(path string) (*ConfusionPairSuite, error) {
	return loadSuite[ConfusionPairSuite](path)
}

// LoadArgumentSuite loads argument correctness tests from a JSON file
func LoadArgumentSuite(path string) (*ArgumentSuite, error) {
	return loadSuite[ArgumentSuite](path)
}

// ToolSelector is an interface that an LLM or mock can implement for testing
type ToolSelector interface {
	// SelectTool returns the tool name and arguments for a given natural language input
	SelectTool(input string) (toolName string, args map[string]interface{}, err error)
}

// EvaluateToolSelection runs tool selection tests against a selector
func EvaluateToolSelection(suite *ToolSelectionSuite, selector ToolSelector) (*EvalMetrics, []ToolSelectionResult) {
	metrics := &EvalMetrics{
		ByCategory: make(map[string]*CategoryMetrics),
		ByTool:     make(map[string]*ToolMetrics),
	}
	var results []ToolSelectionResult

	for _, test := range suite.Tests {
		metrics.TotalTests++

		// Initialize category metrics
		if metrics.ByCategory[test.Category] == nil {
			metrics.ByCategory[test.Category] = &CategoryMetrics{}
		}
		metrics.ByCategory[test.Category].Total++

		// Initialize tool metrics
		if metrics.ByTool[test.ExpectedTool] == nil {
			metrics.ByTool[test.ExpectedTool] = &ToolMetrics{}
		}
		metrics.ByTool[test.ExpectedTool].ExpectedCount++

		// Run the selector
		actualTool, actualArgs, err := selector.SelectTool(test.Input)

		result := ToolSelectionResult{
			TestID:       test.ID,
			Input:        test.Input,
			ExpectedTool: test.ExpectedTool,
			ActualTool:   actualTool,
			Passed:       true,
		}

		if err != nil {
			result.Passed = false
			result.Errors = append(result.Errors, fmt.Sprintf("selector error: %v", err))
		}

		// Check tool selection
		if actualTool != test.ExpectedTool {
			result.Passed = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("wrong tool: expected %s, got %s", test.ExpectedTool, actualTool))
			metrics.ByTool[test.ExpectedTool].FalseNegatives++

			if metrics.ByTool[actualTool] == nil {
				metrics.ByTool[actualTool] = &ToolMetrics{}
			}
			metrics.ByTool[actualTool].FalsePositives++
		} else {
			metrics.ByTool[test.ExpectedTool].CorrectCount++
		}

		// Track selected count
		if metrics.ByTool[actualTool] == nil {
			metrics.ByTool[actualTool] = &ToolMetrics{}
		}
		metrics.ByTool[actualTool].SelectedCount++

		// Check forbidden tools
		for _, forbidden := range test.NotTools {
			if actualTool == forbidden {
				result.Passed = false
				result.Errors = append(result.Errors,
					fmt.Sprintf("selected forbidden tool: %s", forbidden))
			}
		}

		// Check expected arguments
		for key, expectedValue := range test.ExpectedArgs {
			actualValue, exists := actualArgs[key]
			if !exists {
				result.Passed = false
				result.Errors = append(result.Errors,
					fmt.Sprintf("missing arg %s (expected %v)", key, expectedValue))
			} else if !compareValues(expectedValue, actualValue) {
				result.Passed = false
				result.Errors = append(result.Errors,
					fmt.Sprintf("wrong arg %s: expected %v, got %v", key, expectedValue, actualValue))
			}
		}

		// Update metrics
		if result.Passed {
			metrics.PassedTests++
			metrics.ByCategory[test.Category].Passed++
		} else {
			metrics.FailedTests++
			metrics.ByCategory[test.Category].Failed++
			metrics.FailedDetails = append(metrics.FailedDetails,
				fmt.Sprintf("[%s] %s: %s", test.ID, test.Input, strings.Join(result.Errors, "; ")))
		}

		results = append(results, result)
	}

	if metrics.TotalTests > 0 {
		metrics.Accuracy = float64(metrics.PassedTests) / float64(metrics.TotalTests)
	}

	return metrics, results
}

// EvaluateConfusionPairs runs confusion pair tests against a selector
func EvaluateConfusionPairs(suite *ConfusionPairSuite, selector ToolSelector) (*EvalMetrics, []ConfusionPairResult) {
	metrics := &EvalMetrics{
		ByCategory: make(map[string]*CategoryMetrics),
		ByTool:     make(map[string]*ToolMetrics),
	}
	var results []ConfusionPairResult

	for _, pair := range suite.Pairs {
		// Use pair ID as category
		if metrics.ByCategory[pair.ID] == nil {
			metrics.ByCategory[pair.ID] = &CategoryMetrics{}
		}

		for _, test := range pair.Tests {
			metrics.TotalTests++
			metrics.ByCategory[pair.ID].Total++

			// Initialize tool metrics
			if metrics.ByTool[test.Expected] == nil {
				metrics.ByTool[test.Expected] = &ToolMetrics{}
			}
			metrics.ByTool[test.Expected].ExpectedCount++

			// Run the selector
			actualTool, _, err := selector.SelectTool(test.Input)

			result := ConfusionPairResult{
				PairID:       pair.ID,
				TestInput:    test.Input,
				ExpectedTool: test.Expected,
				ActualTool:   actualTool,
				Reason:       test.Reason,
				Passed:       err == nil && actualTool == test.Expected,
			}

			// Track metrics
			if metrics.ByTool[actualTool] == nil {
				metrics.ByTool[actualTool] = &ToolMetrics{}
			}
			metrics.ByTool[actualTool].SelectedCount++

			if result.Passed {
				metrics.PassedTests++
				metrics.ByCategory[pair.ID].Passed++
				metrics.ByTool[test.Expected].CorrectCount++
			} else {
				metrics.FailedTests++
				metrics.ByCategory[pair.ID].Failed++
				metrics.ByTool[test.Expected].FalseNegatives++
				metrics.ByTool[actualTool].FalsePositives++
				metrics.FailedDetails = append(metrics.FailedDetails,
					fmt.Sprintf("[%s] %s: expected %s, got %s (%s)",
						pair.ID, test.Input, test.Expected, actualTool, test.Reason))
			}

			results = append(results, result)
		}
	}

	if metrics.TotalTests > 0 {
		metrics.Accuracy = float64(metrics.PassedTests) / float64(metrics.TotalTests)
	}

	return metrics, results
}

// EvaluateArguments runs argument correctness tests against a selector
func EvaluateArguments(suite *ArgumentSuite, selector ToolSelector) (*EvalMetrics, []ArgumentResult) {
	metrics := &EvalMetrics{
		ByCategory: make(map[string]*CategoryMetrics),
		ByTool:     make(map[string]*ToolMetrics),
	}
	var results []ArgumentResult

	for _, test := range suite.Tests {
		metrics.TotalTests++

		// Use tool name as category
		if metrics.ByCategory[test.Tool] == nil {
			metrics.ByCategory[test.Tool] = &CategoryMetrics{}
		}
		metrics.ByCategory[test.Tool].Total++

		// Run the selector
		actualTool, actualArgs, err := selector.SelectTool(test.Input)

		result := ArgumentResult{
			TestID:    test.ID,
			Tool:      test.Tool,
			Input:     test.Input,
			Passed:    true,
			WrongArgs: make(map[string]string),
		}

		if err != nil {
			result.Passed = false
			continue
		}

		// Check correct tool was selected first
		if actualTool != test.Tool {
			result.Passed = false
			continue
		}

		// Check required arguments
		for _, reqArg := range test.RequiredArgs {
			if _, exists := actualArgs[reqArg]; !exists {
				result.Passed = false
				result.MissingArgs = append(result.MissingArgs, reqArg)
			}
		}

		// Check expected argument values
		for key, expectedValue := range test.ExpectedArgs {
			actualValue, exists := actualArgs[key]
			if !exists {
				result.Passed = false
				result.MissingArgs = append(result.MissingArgs, key)
			} else if !compareValues(expectedValue, actualValue) {
				result.Passed = false
				result.WrongArgs[key] = fmt.Sprintf("expected %v, got %v", expectedValue, actualValue)
			}
		}

		// Check forbidden arguments
		for _, forbidden := range test.ForbiddenArgs {
			if _, exists := actualArgs[forbidden]; exists {
				result.Passed = false
				result.ForbiddenHit = append(result.ForbiddenHit, forbidden)
			}
		}

		// Update metrics
		if result.Passed {
			metrics.PassedTests++
			metrics.ByCategory[test.Tool].Passed++
		} else {
			metrics.FailedTests++
			metrics.ByCategory[test.Tool].Failed++

			var errDetails []string
			if len(result.MissingArgs) > 0 {
				errDetails = append(errDetails, fmt.Sprintf("missing: %v", result.MissingArgs))
			}
			for k, v := range result.WrongArgs {
				errDetails = append(errDetails, fmt.Sprintf("%s: %s", k, v))
			}
			if len(result.ForbiddenHit) > 0 {
				errDetails = append(errDetails, fmt.Sprintf("forbidden: %v", result.ForbiddenHit))
			}
			metrics.FailedDetails = append(metrics.FailedDetails,
				fmt.Sprintf("[%s] %s: %s", test.ID, test.Input, strings.Join(errDetails, "; ")))
		}

		results = append(results, result)
	}

	if metrics.TotalTests > 0 {
		metrics.Accuracy = float64(metrics.PassedTests) / float64(metrics.TotalTests)
	}

	return metrics, results
}

// compareValues compares expected and actual values, handling type differences
func compareValues(expected, actual interface{}) bool {
	// Handle nil cases
	if expected == nil && actual == nil {
		return true
	}
	if expected == nil || actual == nil {
		return false
	}

	// Use reflect for deep comparison
	ev := reflect.ValueOf(expected)
	av := reflect.ValueOf(actual)

	// Handle numeric type differences (JSON unmarshals to float64)
	switch ev.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if av.Kind() == reflect.Float64 {
			return float64(ev.Int()) == av.Float()
		}
	case reflect.Float32, reflect.Float64:
		if av.Kind() == reflect.Float64 {
			return ev.Float() == av.Float()
		}
	}

	// Handle slice comparison
	if ev.Kind() == reflect.Slice && av.Kind() == reflect.Slice {
		if ev.Len() != av.Len() {
			return false
		}
		for i := 0; i < ev.Len(); i++ {
			if !compareValues(ev.Index(i).Interface(), av.Index(i).Interface()) {
				return false
			}
		}
		return true
	}

	// Default: use reflect.DeepEqual
	return reflect.DeepEqual(expected, actual)
}

// FormatMetrics returns a human-readable summary of evaluation metrics
func FormatMetrics(metrics *EvalMetrics, suiteName string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("\n=== %s ===\n", suiteName))
	b.WriteString(fmt.Sprintf("Total: %d tests\n", metrics.TotalTests))
	b.WriteString(fmt.Sprintf("Passed: %d (%.1f%%)\n", metrics.PassedTests, metrics.Accuracy*100))
	b.WriteString(fmt.Sprintf("Failed: %d\n", metrics.FailedTests))

	if len(metrics.ByCategory) > 0 {
		b.WriteString("\nBy Category:\n")
		for cat, m := range metrics.ByCategory {
			if m.Total > 0 {
				acc := float64(m.Passed) / float64(m.Total) * 100
				b.WriteString(fmt.Sprintf("  %-25s: %d/%d (%.0f%%)\n", cat, m.Passed, m.Total, acc))
			}
		}
	}

	if len(metrics.FailedDetails) > 0 && len(metrics.FailedDetails) <= 10 {
		b.WriteString("\nFailed Tests:\n")
		for _, detail := range metrics.FailedDetails {
			b.WriteString(fmt.Sprintf("  - %s\n", detail))
		}
	} else if len(metrics.FailedDetails) > 10 {
		b.WriteString(fmt.Sprintf("\nFailed Tests (showing first 10 of %d):\n", len(metrics.FailedDetails)))
		for _, detail := range metrics.FailedDetails[:10] {
			b.WriteString(fmt.Sprintf("  - %s\n", detail))
		}
	}

	return b.String()
}

// LoadAllEvals loads all evaluation suites from a directory
func LoadAllEvals(dir string) (*ToolSelectionSuite, *ConfusionPairSuite, *ArgumentSuite, error) {
	toolSelection, err := LoadToolSelectionSuite(filepath.Join(dir, "tool_selection.json"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading tool selection: %w", err)
	}

	confusionPairs, err := LoadConfusionPairSuite(filepath.Join(dir, "confusion_pairs.json"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading confusion pairs: %w", err)
	}

	arguments, err := LoadArgumentSuite(filepath.Join(dir, "argument_correctness.json"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading arguments: %w", err)
	}

	return toolSelection, confusionPairs, arguments, nil
}
