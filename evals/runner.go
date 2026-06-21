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

// newMetrics returns an EvalMetrics with its maps initialized.
func newMetrics() *EvalMetrics {
	return &EvalMetrics{
		ByCategory: make(map[string]*CategoryMetrics),
		ByTool:     make(map[string]*ToolMetrics),
	}
}

// categoryMetric returns (creating if needed) the metrics bucket for a category.
func (m *EvalMetrics) categoryMetric(name string) *CategoryMetrics {
	if m.ByCategory[name] == nil {
		m.ByCategory[name] = &CategoryMetrics{}
	}
	return m.ByCategory[name]
}

// toolMetric returns (creating if needed) the metrics bucket for a tool.
func (m *EvalMetrics) toolMetric(name string) *ToolMetrics {
	if m.ByTool[name] == nil {
		m.ByTool[name] = &ToolMetrics{}
	}
	return m.ByTool[name]
}

// finalize computes the accuracy ratio once all tests have been recorded.
func (m *EvalMetrics) finalize() {
	if m.TotalTests > 0 {
		m.Accuracy = float64(m.PassedTests) / float64(m.TotalTests)
	}
}

// EvaluateToolSelection runs tool selection tests against a selector
func EvaluateToolSelection(suite *ToolSelectionSuite, selector ToolSelector) (*EvalMetrics, []ToolSelectionResult) {
	metrics := newMetrics()
	var results []ToolSelectionResult

	for _, test := range suite.Tests {
		metrics.TotalTests++
		metrics.categoryMetric(test.Category).Total++
		metrics.toolMetric(test.ExpectedTool).ExpectedCount++

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

		metrics.recordToolChoice(&test, actualTool, &result)
		result.Errors = append(result.Errors, checkForbiddenTools(test.NotTools, actualTool)...)
		result.Errors = append(result.Errors, checkExpectedArgs(test.ExpectedArgs, actualArgs)...)
		if len(result.Errors) > 0 {
			result.Passed = false
		}

		metrics.recordSelectionOutcome(&test, &result)
		results = append(results, result)
	}

	metrics.finalize()
	return metrics, results
}

// recordToolChoice updates per-tool counters for a selection and notes a
// wrong-tool error on the result if the choice did not match.
func (m *EvalMetrics) recordToolChoice(test *ToolSelectionTest, actualTool string, result *ToolSelectionResult) {
	if actualTool != test.ExpectedTool {
		result.Errors = append(result.Errors,
			fmt.Sprintf("wrong tool: expected %s, got %s", test.ExpectedTool, actualTool))
		m.toolMetric(test.ExpectedTool).FalseNegatives++
		m.toolMetric(actualTool).FalsePositives++
	} else {
		m.toolMetric(test.ExpectedTool).CorrectCount++
	}
	m.toolMetric(actualTool).SelectedCount++
}

// recordSelectionOutcome updates pass/fail aggregates for a tool-selection test.
func (m *EvalMetrics) recordSelectionOutcome(test *ToolSelectionTest, result *ToolSelectionResult) {
	cat := m.categoryMetric(test.Category)
	if result.Passed {
		m.PassedTests++
		cat.Passed++
		return
	}
	m.FailedTests++
	cat.Failed++
	m.FailedDetails = append(m.FailedDetails,
		fmt.Sprintf("[%s] %s: %s", test.ID, test.Input, strings.Join(result.Errors, "; ")))
}

// checkForbiddenTools returns an error string for each forbidden tool that was
// selected.
func checkForbiddenTools(forbidden []string, actualTool string) []string {
	var errs []string
	for _, name := range forbidden {
		if actualTool == name {
			errs = append(errs, fmt.Sprintf("selected forbidden tool: %s", name))
		}
	}
	return errs
}

// checkExpectedArgs returns an error string for each expected argument that is
// missing or has the wrong value.
func checkExpectedArgs(expected map[string]interface{}, actual map[string]interface{}) []string {
	var errs []string
	for key, expectedValue := range expected {
		actualValue, exists := actual[key]
		switch {
		case !exists:
			errs = append(errs, fmt.Sprintf("missing arg %s (expected %v)", key, expectedValue))
		case !compareValues(expectedValue, actualValue):
			errs = append(errs, fmt.Sprintf("wrong arg %s: expected %v, got %v", key, expectedValue, actualValue))
		}
	}
	return errs
}

// EvaluateConfusionPairs runs confusion pair tests against a selector
func EvaluateConfusionPairs(suite *ConfusionPairSuite, selector ToolSelector) (*EvalMetrics, []ConfusionPairResult) {
	metrics := newMetrics()
	var results []ConfusionPairResult

	for _, pair := range suite.Pairs {
		metrics.categoryMetric(pair.ID)
		for _, test := range pair.Tests {
			metrics.TotalTests++
			metrics.categoryMetric(pair.ID).Total++
			metrics.toolMetric(test.Expected).ExpectedCount++

			actualTool, _, err := selector.SelectTool(test.Input)
			result := ConfusionPairResult{
				PairID:       pair.ID,
				TestInput:    test.Input,
				ExpectedTool: test.Expected,
				ActualTool:   actualTool,
				Reason:       test.Reason,
				Passed:       err == nil && actualTool == test.Expected,
			}
			metrics.toolMetric(actualTool).SelectedCount++
			metrics.recordConfusionOutcome(pair.ID, &test, &result)
			results = append(results, result)
		}
	}

	metrics.finalize()
	return metrics, results
}

// recordConfusionOutcome updates pass/fail aggregates for a confusion-pair test.
func (m *EvalMetrics) recordConfusionOutcome(pairID string, test *ConfusionPairTest, result *ConfusionPairResult) {
	cat := m.categoryMetric(pairID)
	if result.Passed {
		m.PassedTests++
		cat.Passed++
		m.toolMetric(test.Expected).CorrectCount++
		return
	}
	m.FailedTests++
	cat.Failed++
	m.toolMetric(test.Expected).FalseNegatives++
	m.toolMetric(result.ActualTool).FalsePositives++
	m.FailedDetails = append(m.FailedDetails,
		fmt.Sprintf("[%s] %s: expected %s, got %s (%s)",
			pairID, test.Input, test.Expected, result.ActualTool, test.Reason))
}

// EvaluateArguments runs argument correctness tests against a selector
func EvaluateArguments(suite *ArgumentSuite, selector ToolSelector) (*EvalMetrics, []ArgumentResult) {
	metrics := newMetrics()
	var results []ArgumentResult

	for _, test := range suite.Tests {
		metrics.TotalTests++
		metrics.categoryMetric(test.Tool).Total++

		result, recorded := evaluateArgumentTest(&test, selector)
		// Preserve original behavior: a selector error or wrong-tool
		// short-circuit increments TotalTests/category Total only (above);
		// it is neither tallied as pass/fail nor appended to results.
		if recorded {
			metrics.recordArgumentOutcome(&test, &result)
			results = append(results, result)
		}
	}

	metrics.finalize()
	return metrics, results
}

// evaluateArgumentTest runs a single argument-correctness test and returns its
// result. The recorded flag is false when the test short-circuited on a
// selector error or wrong tool, in which case the caller omits it from the
// detailed results slice (matching the original control flow).
func evaluateArgumentTest(test *ArgumentTest, selector ToolSelector) (result ArgumentResult, recorded bool) {
	result = ArgumentResult{
		TestID:    test.ID,
		Tool:      test.Tool,
		Input:     test.Input,
		Passed:    true,
		WrongArgs: make(map[string]string),
	}

	actualTool, actualArgs, err := selector.SelectTool(test.Input)
	if err != nil || actualTool != test.Tool {
		result.Passed = false
		return result, false
	}

	checkRequiredArgs(test.RequiredArgs, actualArgs, &result)
	checkExpectedArgValues(test.ExpectedArgs, actualArgs, &result)
	checkForbiddenArgs(test.ForbiddenArgs, actualArgs, &result)
	return result, true
}

// checkRequiredArgs marks missing required arguments on the result.
func checkRequiredArgs(required []string, actual map[string]interface{}, result *ArgumentResult) {
	for _, reqArg := range required {
		if _, exists := actual[reqArg]; !exists {
			result.Passed = false
			result.MissingArgs = append(result.MissingArgs, reqArg)
		}
	}
}

// checkExpectedArgValues marks missing or wrong-valued expected arguments.
func checkExpectedArgValues(expected map[string]interface{}, actual map[string]interface{}, result *ArgumentResult) {
	for key, expectedValue := range expected {
		actualValue, exists := actual[key]
		switch {
		case !exists:
			result.Passed = false
			result.MissingArgs = append(result.MissingArgs, key)
		case !compareValues(expectedValue, actualValue):
			result.Passed = false
			result.WrongArgs[key] = fmt.Sprintf("expected %v, got %v", expectedValue, actualValue)
		}
	}
}

// checkForbiddenArgs marks any forbidden argument that was supplied.
func checkForbiddenArgs(forbidden []string, actual map[string]interface{}, result *ArgumentResult) {
	for _, name := range forbidden {
		if _, exists := actual[name]; exists {
			result.Passed = false
			result.ForbiddenHit = append(result.ForbiddenHit, name)
		}
	}
}

// recordArgumentOutcome updates pass/fail aggregates for an argument test.
func (m *EvalMetrics) recordArgumentOutcome(test *ArgumentTest, result *ArgumentResult) {
	cat := m.categoryMetric(test.Tool)
	if result.Passed {
		m.PassedTests++
		cat.Passed++
		return
	}
	m.FailedTests++
	cat.Failed++
	m.FailedDetails = append(m.FailedDetails,
		fmt.Sprintf("[%s] %s: %s", test.ID, test.Input, formatArgErrors(result)))
}

// formatArgErrors renders the missing/wrong/forbidden detail for a failed
// argument test into a single semicolon-joined string.
func formatArgErrors(result *ArgumentResult) string {
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
	return strings.Join(errDetails, "; ")
}

// compareValues compares expected and actual values, handling type differences
func compareValues(expected, actual interface{}) bool {
	if expected == nil || actual == nil {
		return expected == nil && actual == nil
	}

	ev := reflect.ValueOf(expected)
	av := reflect.ValueOf(actual)

	if matched, equal := compareNumeric(ev, av); matched {
		return equal
	}
	if ev.Kind() == reflect.Slice && av.Kind() == reflect.Slice {
		return compareSlices(ev, av)
	}
	return reflect.DeepEqual(expected, actual)
}

// compareNumeric handles the JSON-unmarshals-numbers-to-float64 case. It
// returns matched=true when expected is a numeric kind, along with whether the
// two values are equal.
func compareNumeric(ev, av reflect.Value) (matched, equal bool) {
	if av.Kind() != reflect.Float64 {
		return false, false
	}
	switch ev.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true, float64(ev.Int()) == av.Float()
	case reflect.Float32, reflect.Float64:
		return true, ev.Float() == av.Float()
	default:
		return false, false
	}
}

// compareSlices compares two slices element-by-element via compareValues.
func compareSlices(ev, av reflect.Value) bool {
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

// FormatMetrics returns a human-readable summary of evaluation metrics
func FormatMetrics(metrics *EvalMetrics, suiteName string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("\n=== %s ===\n", suiteName))
	b.WriteString(fmt.Sprintf("Total: %d tests\n", metrics.TotalTests))
	b.WriteString(fmt.Sprintf("Passed: %d (%.1f%%)\n", metrics.PassedTests, metrics.Accuracy*100))
	b.WriteString(fmt.Sprintf("Failed: %d\n", metrics.FailedTests))

	writeCategoryBreakdown(&b, metrics.ByCategory)
	writeFailedDetails(&b, metrics.FailedDetails)

	return b.String()
}

// writeCategoryBreakdown appends a per-category accuracy table to b.
func writeCategoryBreakdown(b *strings.Builder, byCategory map[string]*CategoryMetrics) {
	if len(byCategory) == 0 {
		return
	}
	b.WriteString("\nBy Category:\n")
	for cat, m := range byCategory {
		if m.Total > 0 {
			acc := float64(m.Passed) / float64(m.Total) * 100
			b.WriteString(fmt.Sprintf("  %-25s: %d/%d (%.0f%%)\n", cat, m.Passed, m.Total, acc))
		}
	}
}

// writeFailedDetails appends the failed-test list to b, capped at 10 entries.
func writeFailedDetails(b *strings.Builder, details []string) {
	if len(details) == 0 {
		return
	}
	shown := details
	if len(details) > 10 {
		b.WriteString(fmt.Sprintf("\nFailed Tests (showing first 10 of %d):\n", len(details)))
		shown = details[:10]
	} else {
		b.WriteString("\nFailed Tests:\n")
	}
	for _, detail := range shown {
		b.WriteString(fmt.Sprintf("  - %s\n", detail))
	}
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
