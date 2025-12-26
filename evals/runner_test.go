package evals

import (
	"path/filepath"
	"testing"
)

// MockToolSelector implements ToolSelector for testing
type MockToolSelector struct {
	// Responses maps input strings to tool selections
	Responses map[string]struct {
		Tool string
		Args map[string]interface{}
	}
	// DefaultTool is returned if input isn't in Responses
	DefaultTool string
}

func (m *MockToolSelector) SelectTool(input string) (string, map[string]interface{}, error) {
	if resp, ok := m.Responses[input]; ok {
		return resp.Tool, resp.Args, nil
	}
	return m.DefaultTool, nil, nil
}

// PerfectToolSelector returns the expected tool for each test
type PerfectToolSelector struct {
	suite *ToolSelectionSuite
}

func (p *PerfectToolSelector) SelectTool(input string) (string, map[string]interface{}, error) {
	for _, test := range p.suite.Tests {
		if test.Input == input {
			return test.ExpectedTool, test.ExpectedArgs, nil
		}
	}
	return "", nil, nil
}

func TestLoadToolSelectionSuite(t *testing.T) {
	suite, err := LoadToolSelectionSuite(filepath.Join(".", "tool_selection.json"))
	if err != nil {
		t.Fatalf("Failed to load tool selection suite: %v", err)
	}

	if suite.Name == "" {
		t.Error("Suite name should not be empty")
	}

	if len(suite.Tests) == 0 {
		t.Error("Suite should have tests")
	}

	// Check first test has required fields
	if len(suite.Tests) > 0 {
		test := suite.Tests[0]
		if test.ID == "" {
			t.Error("Test ID should not be empty")
		}
		if test.Input == "" {
			t.Error("Test input should not be empty")
		}
		if test.ExpectedTool == "" {
			t.Error("Expected tool should not be empty")
		}
	}
}

func TestLoadConfusionPairSuite(t *testing.T) {
	suite, err := LoadConfusionPairSuite(filepath.Join(".", "confusion_pairs.json"))
	if err != nil {
		t.Fatalf("Failed to load confusion pair suite: %v", err)
	}

	if suite.Name == "" {
		t.Error("Suite name should not be empty")
	}

	if len(suite.Pairs) == 0 {
		t.Error("Suite should have confusion pairs")
	}

	// Check first pair has required fields
	if len(suite.Pairs) > 0 {
		pair := suite.Pairs[0]
		if pair.ID == "" {
			t.Error("Pair ID should not be empty")
		}
		if len(pair.Tools) < 2 {
			t.Error("Pair should have at least 2 tools")
		}
		if len(pair.Tests) == 0 {
			t.Error("Pair should have tests")
		}
	}
}

func TestLoadArgumentSuite(t *testing.T) {
	suite, err := LoadArgumentSuite(filepath.Join(".", "argument_correctness.json"))
	if err != nil {
		t.Fatalf("Failed to load argument suite: %v", err)
	}

	if suite.Name == "" {
		t.Error("Suite name should not be empty")
	}

	if len(suite.Tests) == 0 {
		t.Error("Suite should have tests")
	}

	// Check first test has required fields
	if len(suite.Tests) > 0 {
		test := suite.Tests[0]
		if test.ID == "" {
			t.Error("Test ID should not be empty")
		}
		if test.Tool == "" {
			t.Error("Test tool should not be empty")
		}
		if test.Input == "" {
			t.Error("Test input should not be empty")
		}
	}
}

func TestEvaluateToolSelection(t *testing.T) {
	suite, err := LoadToolSelectionSuite(filepath.Join(".", "tool_selection.json"))
	if err != nil {
		t.Fatalf("Failed to load suite: %v", err)
	}

	// Test with perfect selector (should get 100% accuracy)
	perfectSelector := &PerfectToolSelector{suite: suite}
	metrics, results := EvaluateToolSelection(suite, perfectSelector)

	if metrics.TotalTests != len(suite.Tests) {
		t.Errorf("Total tests: expected %d, got %d", len(suite.Tests), metrics.TotalTests)
	}

	if metrics.Accuracy != 1.0 {
		t.Errorf("Perfect selector should have 100%% accuracy, got %.1f%%", metrics.Accuracy*100)
	}

	if len(results) != len(suite.Tests) {
		t.Errorf("Should have result for each test")
	}

	// All results should pass
	for _, result := range results {
		if !result.Passed {
			t.Errorf("Test %s should pass with perfect selector", result.TestID)
		}
	}
}

func TestEvaluateToolSelectionWithWrongAnswers(t *testing.T) {
	suite := &ToolSelectionSuite{
		Name: "Test Suite",
		Tests: []ToolSelectionTest{
			{
				ID:           "test-001",
				Category:     "search",
				Input:        "find pages about authentication",
				ExpectedTool: "mediawiki_search",
				ExpectedArgs: map[string]interface{}{"query": "authentication"},
				NotTools:     []string{"mediawiki_get_page"},
			},
			{
				ID:           "test-002",
				Category:     "read",
				Input:        "show me the Main Page",
				ExpectedTool: "mediawiki_get_page",
				ExpectedArgs: map[string]interface{}{"title": "Main Page"},
			},
		},
	}

	// Mock selector that always returns wrong tool
	wrongSelector := &MockToolSelector{
		DefaultTool: "mediawiki_edit_page",
	}

	metrics, results := EvaluateToolSelection(suite, wrongSelector)

	if metrics.PassedTests != 0 {
		t.Errorf("Wrong selector should have 0 passed tests, got %d", metrics.PassedTests)
	}

	if metrics.FailedTests != 2 {
		t.Errorf("Wrong selector should have 2 failed tests, got %d", metrics.FailedTests)
	}

	if metrics.Accuracy != 0 {
		t.Errorf("Wrong selector should have 0%% accuracy, got %.1f%%", metrics.Accuracy*100)
	}

	for _, result := range results {
		if result.Passed {
			t.Errorf("Test %s should not pass with wrong selector", result.TestID)
		}
		if len(result.Errors) == 0 {
			t.Errorf("Test %s should have errors", result.TestID)
		}
	}
}

func TestEvaluateConfusionPairs(t *testing.T) {
	suite := &ConfusionPairSuite{
		Name: "Test Confusion Pairs",
		Pairs: []ConfusionPair{
			{
				ID:             "pair-search",
				Tools:          []string{"mediawiki_search", "mediawiki_search_in_page"},
				Disambiguation: "search = across wiki, search_in_page = within known page",
				Tests: []ConfusionPairTest{
					{
						Input:    "find documentation about OAuth",
						Expected: "mediawiki_search",
						Reason:   "Unknown page location",
					},
					{
						Input:    "search for OAuth on the Authentication page",
						Expected: "mediawiki_search_in_page",
						Reason:   "Specific page specified",
					},
				},
			},
		},
	}

	// Perfect selector for confusion pairs
	perfectSelector := &MockToolSelector{
		Responses: map[string]struct {
			Tool string
			Args map[string]interface{}
		}{
			"find documentation about OAuth": {
				Tool: "mediawiki_search",
				Args: map[string]interface{}{"query": "OAuth"},
			},
			"search for OAuth on the Authentication page": {
				Tool: "mediawiki_search_in_page",
				Args: map[string]interface{}{"title": "Authentication", "query": "OAuth"},
			},
		},
	}

	metrics, results := EvaluateConfusionPairs(suite, perfectSelector)

	if metrics.TotalTests != 2 {
		t.Errorf("Expected 2 tests, got %d", metrics.TotalTests)
	}

	if metrics.Accuracy != 1.0 {
		t.Errorf("Perfect selector should have 100%% accuracy, got %.1f%%", metrics.Accuracy*100)
	}

	for _, result := range results {
		if !result.Passed {
			t.Errorf("Test should pass: %s", result.TestInput)
		}
	}
}

func TestEvaluateArguments(t *testing.T) {
	suite := &ArgumentSuite{
		Name: "Test Arguments",
		Tests: []ArgumentTest{
			{
				ID:           "args-001",
				Tool:         "mediawiki_search",
				Input:        "find pages about authentication with limit 20",
				RequiredArgs: []string{"query"},
				ExpectedArgs: map[string]interface{}{
					"query": "authentication",
					"limit": float64(20), // JSON numbers are float64
				},
				ForbiddenArgs: []string{"title"},
			},
		},
	}

	// Correct selector
	correctSelector := &MockToolSelector{
		Responses: map[string]struct {
			Tool string
			Args map[string]interface{}
		}{
			"find pages about authentication with limit 20": {
				Tool: "mediawiki_search",
				Args: map[string]interface{}{
					"query": "authentication",
					"limit": float64(20),
				},
			},
		},
	}

	metrics, results := EvaluateArguments(suite, correctSelector)

	if metrics.TotalTests != 1 {
		t.Errorf("Expected 1 test, got %d", metrics.TotalTests)
	}

	if metrics.PassedTests != 1 {
		t.Errorf("Expected 1 passed test, got %d", metrics.PassedTests)
	}

	if len(results) > 0 && !results[0].Passed {
		t.Errorf("Test should pass: missing=%v, wrong=%v, forbidden=%v",
			results[0].MissingArgs, results[0].WrongArgs, results[0].ForbiddenHit)
	}
}

func TestEvaluateArgumentsWithForbidden(t *testing.T) {
	suite := &ArgumentSuite{
		Name: "Test Forbidden Args",
		Tests: []ArgumentTest{
			{
				ID:            "args-001",
				Tool:          "mediawiki_search",
				Input:         "find pages about authentication",
				RequiredArgs:  []string{"query"},
				ExpectedArgs:  map[string]interface{}{"query": "authentication"},
				ForbiddenArgs: []string{"title"},
			},
		},
	}

	// Selector that includes forbidden arg
	badSelector := &MockToolSelector{
		Responses: map[string]struct {
			Tool string
			Args map[string]interface{}
		}{
			"find pages about authentication": {
				Tool: "mediawiki_search",
				Args: map[string]interface{}{
					"query": "authentication",
					"title": "some title", // forbidden!
				},
			},
		},
	}

	metrics, results := EvaluateArguments(suite, badSelector)

	if metrics.PassedTests != 0 {
		t.Errorf("Expected 0 passed tests (forbidden arg used), got %d", metrics.PassedTests)
	}

	if len(results) > 0 && len(results[0].ForbiddenHit) == 0 {
		t.Error("Should flag forbidden arg usage")
	}
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		name     string
		expected interface{}
		actual   interface{}
		want     bool
	}{
		{"equal strings", "test", "test", true},
		{"different strings", "test", "other", false},
		{"int vs float64", 20, float64(20), true},
		{"equal slices", []string{"a", "b"}, []string{"a", "b"}, true},
		{"different slices", []string{"a", "b"}, []string{"a", "c"}, false},
		{"nil values", nil, nil, true},
		{"nil vs value", nil, "test", false},
		{"equal bools", true, true, true},
		{"different bools", true, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareValues(tt.expected, tt.actual)
			if got != tt.want {
				t.Errorf("compareValues(%v, %v) = %v, want %v", tt.expected, tt.actual, got, tt.want)
			}
		})
	}
}

func TestFormatMetrics(t *testing.T) {
	metrics := &EvalMetrics{
		TotalTests:  10,
		PassedTests: 8,
		FailedTests: 2,
		Accuracy:    0.8,
		ByCategory: map[string]*CategoryMetrics{
			"search": {Total: 5, Passed: 4, Failed: 1},
			"read":   {Total: 5, Passed: 4, Failed: 1},
		},
		FailedDetails: []string{
			"[test-1] input: error",
			"[test-2] input: error",
		},
	}

	output := FormatMetrics(metrics, "Test Suite")

	if output == "" {
		t.Error("FormatMetrics should return non-empty string")
	}

	// Check key info is present
	if !contains(output, "80") { // 80%
		t.Error("Should show accuracy percentage")
	}
	if !contains(output, "search") {
		t.Error("Should show category breakdown")
	}
	if !contains(output, "Failed Tests") {
		t.Error("Should show failed tests section")
	}
}

func TestLoadAllEvals(t *testing.T) {
	toolSelection, confusionPairs, arguments, err := LoadAllEvals(".")
	if err != nil {
		t.Fatalf("Failed to load all evals: %v", err)
	}

	if toolSelection == nil {
		t.Fatal("Tool selection suite should not be nil")
	}
	if confusionPairs == nil {
		t.Fatal("Confusion pairs suite should not be nil")
	}
	if arguments == nil {
		t.Fatal("Arguments suite should not be nil")
	}

	// Count total tests
	total := len(toolSelection.Tests)
	for _, pair := range confusionPairs.Pairs {
		total += len(pair.Tests)
	}
	total += len(arguments.Tests)

	t.Logf("Loaded %d total evaluation tests", total)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
