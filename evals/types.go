package evals

// ToolSelectionTest represents a single tool selection evaluation case
type ToolSelectionTest struct {
	ID           string                 `json:"id"`
	Category     string                 `json:"category"`
	Input        string                 `json:"input"`
	ExpectedTool string                 `json:"expected_tool"`
	ExpectedArgs map[string]interface{} `json:"expected_args"`
	NotTools     []string               `json:"not_tools"`
}

// ToolSelectionSuite contains all tool selection tests
type ToolSelectionSuite struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Description string              `json:"description"`
	Tests       []ToolSelectionTest `json:"tests"`
}

// ConfusionPairTest represents a single disambiguation test
type ConfusionPairTest struct {
	Input    string `json:"input"`
	Expected string `json:"expected"`
	Reason   string `json:"reason"`
}

// ConfusionPair represents a pair of tools that are commonly confused
type ConfusionPair struct {
	ID             string              `json:"id"`
	Tools          []string            `json:"tools"`
	Disambiguation string              `json:"disambiguation"`
	Tests          []ConfusionPairTest `json:"tests"`
}

// ConfusionPairSuite contains all confusion pair tests
type ConfusionPairSuite struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Pairs       []ConfusionPair `json:"pairs"`
}

// ArgumentTest represents a single argument correctness test
type ArgumentTest struct {
	ID            string                 `json:"id"`
	Tool          string                 `json:"tool"`
	Input         string                 `json:"input"`
	RequiredArgs  []string               `json:"required_args"`
	ExpectedArgs  map[string]interface{} `json:"expected_args"`
	ForbiddenArgs []string               `json:"forbidden_args"`
	ArgNotes      string                 `json:"arg_notes,omitempty"`
}

// ValidationRules defines common validation rules for arguments
type ValidationRules struct {
	TitleFormat     string `json:"title_format"`
	CategoryFormat  string `json:"category_format"`
	BooleanHandling string `json:"boolean_handling"`
	ArrayHandling   string `json:"array_handling"`
	PreviewDefault  string `json:"preview_default"`
}

// ArgumentSuite contains all argument correctness tests
type ArgumentSuite struct {
	Name            string          `json:"name"`
	Version         string          `json:"version"`
	Description     string          `json:"description"`
	Tests           []ArgumentTest  `json:"tests"`
	ValidationRules ValidationRules `json:"validation_rules"`
}

// ToolSelectionResult represents the result of a single tool selection evaluation
type ToolSelectionResult struct {
	TestID       string
	Input        string
	ExpectedTool string
	ActualTool   string
	Passed       bool
	Errors       []string
}

// ConfusionPairResult represents the result of a confusion pair evaluation
type ConfusionPairResult struct {
	PairID       string
	TestInput    string
	ExpectedTool string
	ActualTool   string
	Reason       string
	Passed       bool
}

// ArgumentResult represents the result of an argument correctness evaluation
type ArgumentResult struct {
	TestID       string
	Tool         string
	Input        string
	Passed       bool
	MissingArgs  []string
	WrongArgs    map[string]string // arg -> "expected X, got Y"
	ForbiddenHit []string          // forbidden args that were used
}

// EvalMetrics contains aggregate metrics for an evaluation run
type EvalMetrics struct {
	TotalTests    int
	PassedTests   int
	FailedTests   int
	Accuracy      float64 // PassedTests / TotalTests
	ByCategory    map[string]*CategoryMetrics
	ByTool        map[string]*ToolMetrics
	FailedDetails []string
}

// CategoryMetrics contains metrics per category
type CategoryMetrics struct {
	Total  int
	Passed int
	Failed int
}

// ToolMetrics contains metrics per tool
type ToolMetrics struct {
	ExpectedCount  int // times tool was expected
	SelectedCount  int // times tool was actually selected
	CorrectCount   int // times tool was correctly selected
	FalsePositives int // times wrong tool was selected instead
	FalseNegatives int // times this tool should have been selected but wasn't
}
