package wiki

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseWikiTableGlossary(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantCount int
		wantTerms []GlossaryTerm
	}{
		{
			name: "parses simple wikitable",
			content: `{| class="wikitable"
|-
! Incorrect !! Correct !! Pattern !! Notes
|-
| Avanade || Avanade Inc. || || Company name
|-
| avanade || Avanade Inc. || || Lowercase
|}`,
			wantCount: 2,
			wantTerms: []GlossaryTerm{
				{Incorrect: "Avanade", Correct: "Avanade Inc.", Pattern: "Company name", Notes: ""},
				{Incorrect: "avanade", Correct: "Avanade Inc.", Pattern: "Lowercase", Notes: ""},
			},
		},
		{
			name: "parses mcp-glossary class table",
			content: `{| class="mcp-glossary wikitable"
|-
| backend || back-end ||  || Hyphenated form preferred
|}`,
			wantCount: 1,
			wantTerms: []GlossaryTerm{
				{Incorrect: "backend", Correct: "back-end", Pattern: "Hyphenated form preferred", Notes: ""},
			},
		},
		{
			name: "handles double-pipe cell separators",
			content: `{| class="wikitable"
|-
| Cell1 || Cell2 || Cell3 || Cell4
|}`,
			wantCount: 1,
		},
		{
			name: "skips header rows",
			content: `{| class="wikitable"
|-
! Header1 !! Header2 !! Header3
|-
| value1 || value2 || value3
|}`,
			wantCount: 1,
		},
		{
			name: "skips empty rows",
			content: `{| class="wikitable"
|-

|-
| value1 || value2
|}`,
			wantCount: 1,
		},
		{
			name: "skips terms where incorrect equals correct",
			content: `{| class="wikitable"
|-
| same || same
|}`,
			wantCount: 0,
		},
		{
			name: "skips terms with empty incorrect",
			content: `{| class="wikitable"
|-
|  || correct value
|}`,
			wantCount: 0,
		},
		{
			name:      "handles empty content",
			content:   "",
			wantCount: 0,
		},
		{
			name:      "handles content without tables",
			content:   "Just some regular text without any wiki tables",
			wantCount: 0,
		},
		{
			name: "handles table without wikitable class",
			content: `{| class="sometable"
|-
| value1 || value2
|}`,
			wantCount: 0,
		},
		{
			name: "extracts pattern column",
			content: `{| class="wikitable"
|-
| typo || correct || \btypo\b ||
|}`,
			wantCount: 1,
			wantTerms: []GlossaryTerm{
				{Incorrect: "typo", Correct: "correct", Pattern: `\btypo\b`, Notes: ""},
			},
		},
		{
			name: "handles multiple tables",
			content: `{| class="wikitable"
|-
| term1 || correct1
|}
Some text between
{| class="wikitable"
|-
| term2 || correct2
|}`,
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			terms := parseWikiTableGlossary(tt.content)
			if len(terms) != tt.wantCount {
				t.Errorf("parseWikiTableGlossary() returned %d terms, want %d", len(terms), tt.wantCount)
				for i, term := range terms {
					t.Logf("  term[%d]: %+v", i, term)
				}
			}
			if tt.wantTerms != nil {
				for i, want := range tt.wantTerms {
					if i >= len(terms) {
						t.Errorf("missing term[%d]: want %+v", i, want)
						continue
					}
					got := terms[i]
					if got.Incorrect != want.Incorrect {
						t.Errorf("term[%d].Incorrect = %q, want %q", i, got.Incorrect, want.Incorrect)
					}
					if got.Correct != want.Correct {
						t.Errorf("term[%d].Correct = %q, want %q", i, got.Correct, want.Correct)
					}
					if got.Pattern != want.Pattern {
						t.Errorf("term[%d].Pattern = %q, want %q", i, got.Pattern, want.Pattern)
					}
					if got.Notes != want.Notes {
						t.Errorf("term[%d].Notes = %q, want %q", i, got.Notes, want.Notes)
					}
				}
			}
		})
	}
}

func TestParseTableRow(t *testing.T) {
	tests := []struct {
		name      string
		row       string
		wantCells []string
	}{
		{
			name:      "parses double-pipe separated cells",
			row:       "| cell1 || cell2 || cell3",
			wantCells: []string{"cell1", "cell2", "cell3"},
		},
		{
			name:      "parses cells on separate lines",
			row:       "| cell1\n| cell2\n| cell3",
			wantCells: []string{"cell1", "cell2", "cell3"},
		},
		{
			name:      "handles mixed format",
			row:       "| cell1 || cell2\n| cell3",
			wantCells: []string{"cell1", "cell2", "cell3"},
		},
		{
			name:      "trims whitespace",
			row:       "|   cell1   ||   cell2   ",
			wantCells: []string{"cell1", "cell2"},
		},
		{
			name:      "skips empty cells",
			row:       "| cell1 || || cell3",
			wantCells: []string{"cell1", "cell3"},
		},
		{
			name:      "skips header lines",
			row:       "! Header1 !! Header2",
			wantCells: []string{},
		},
		{
			name:      "handles empty row",
			row:       "",
			wantCells: []string{},
		},
		{
			name:      "handles row with only whitespace",
			row:       "   \n   ",
			wantCells: []string{},
		},
		{
			name:      "handles cells without leading pipe",
			row:       "cell1 || cell2",
			wantCells: []string{"cell1", "cell2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cells := parseTableRow(tt.row)
			if len(cells) != len(tt.wantCells) {
				t.Errorf("parseTableRow() returned %d cells, want %d: got %v", len(cells), len(tt.wantCells), cells)
				return
			}
			for i, want := range tt.wantCells {
				if cells[i] != want {
					t.Errorf("parseTableRow()[%d] = %q, want %q", i, cells[i], want)
				}
			}
		})
	}
}

func TestExtractContext(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		start      int
		end        int
		contextLen int
		want       string
	}{
		{
			name:       "extracts context around match",
			line:       "This is some text with a keyword in the middle of it",
			start:      25,
			end:        32, // "keyword"
			contextLen: 10,
			want:       "...xt with a keyword in the mi...",
		},
		{
			name:       "no prefix ellipsis at start",
			line:       "keyword at the start",
			start:      0,
			end:        7,
			contextLen: 10,
			want:       "keyword at the st...",
		},
		{
			name:       "no suffix ellipsis at end",
			line:       "text ending with keyword",
			start:      17,
			end:        24,
			contextLen: 10,
			want:       "...ding with keyword",
		},
		{
			name:       "short line no ellipsis",
			line:       "short keyword",
			start:      6,
			end:        13,
			contextLen: 20,
			want:       "short keyword",
		},
		{
			name:       "handles zero context length",
			line:       "some keyword here",
			start:      5,
			end:        12,
			contextLen: 0,
			want:       "...keyword...",
		},
		{
			name:       "handles match at exact boundaries",
			line:       "abc",
			start:      0,
			end:        3,
			contextLen: 5,
			want:       "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractContext(tt.line, tt.start, tt.end, tt.contextLen)
			if got != tt.want {
				t.Errorf("extractContext() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripCodeBlocksForTerminology(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		contains string
		excludes string
	}{
		{
			name:     "removes syntaxhighlight content",
			content:  "Before <syntaxhighlight>SI.Data.Class</syntaxhighlight> After",
			contains: "Before",
			excludes: "SI.Data",
		},
		{
			name:     "removes source tag content",
			content:  "Text <source lang=\"python\">import SI.Data</source> more",
			contains: "Text",
			excludes: "import",
		},
		{
			name:     "removes pre tag content",
			content:  "Before <pre>namespace.Method()</pre> After",
			contains: "Before",
			excludes: "namespace",
		},
		{
			name:     "removes code tag content",
			content:  "Use <code>API.endpoint</code> here",
			contains: "Use",
			excludes: "API",
		},
		{
			name:     "preserves line count",
			content:  "Line1\n<code>Line2\nLine3</code>\nLine4",
			contains: "Line1",
			excludes: "", // just check structure is preserved
		},
		{
			name:     "handles nested newlines",
			content:  "Start\n<syntaxhighlight>\ncode\nmore code\n</syntaxhighlight>\nEnd",
			contains: "Start",
			excludes: "more code",
		},
		{
			name:     "handles case insensitive tags",
			content:  "Text <CODE>inner</CODE> more",
			contains: "Text",
			excludes: "inner",
		},
		{
			name:     "handles content without code blocks",
			content:  "Just regular content here",
			contains: "Just regular content here",
			excludes: "",
		},
		{
			name:     "handles empty content",
			content:  "",
			contains: "",
			excludes: "",
		},
		{
			name:     "handles multiple code blocks",
			content:  "A <code>x</code> B <pre>y</pre> C",
			contains: "A",
			excludes: "x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripCodeBlocksForTerminology(tt.content)
			if tt.contains != "" && !containsString(result, tt.contains) {
				t.Errorf("stripCodeBlocksForTerminology() should contain %q, got %q", tt.contains, result)
			}
			if tt.excludes != "" && containsString(result, tt.excludes) {
				t.Errorf("stripCodeBlocksForTerminology() should NOT contain %q, got %q", tt.excludes, result)
			}
		})
	}
}

func TestStripCodeBlocksForTerminology_PreservesLineNumbers(t *testing.T) {
	content := "Line 1\n<code>\nLine 3 code\nLine 4 code\n</code>\nLine 6"
	result := stripCodeBlocksForTerminology(content)

	// Count newlines - should be preserved
	originalNewlines := countNewlines(content)
	resultNewlines := countNewlines(result)

	if originalNewlines != resultNewlines {
		t.Errorf("Newline count changed from %d to %d", originalNewlines, resultNewlines)
	}
}

// Helper functions

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if len(substr) == 0 {
			return true
		}
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return true
		}
	}
	return len(substr) == 0
}

func countNewlines(s string) int {
	count := 0
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
}

func TestCheckTerminology_NoPagesOrCategory(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Return empty response
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.CheckTerminology(ctx, CheckTerminologyArgs{
		// No pages or category specified
	})

	if err == nil {
		t.Fatal("Expected error for missing pages/category")
	}
}

func TestCheckTerminology_Success(t *testing.T) {
	requestCount := 0
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		requestCount++

		if action == "query" {
			titles := r.FormValue("titles")
			// Glossary page request
			if titles == "Brand Terminology Glossary" {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  "Brand Terminology Glossary",
								"revisions": []interface{}{
									map[string]interface{}{
										"slots": map[string]interface{}{
											"main": map[string]interface{}{
												"*": `{| class="wikitable"
|-
! Incorrect !! Correct
|-
| publc || public
|}`,
											},
										},
									},
								},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
			// Content page request
			if titles == "Test Page" {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"2": map[string]interface{}{
								"pageid": float64(2),
								"title":  "Test Page",
								"revisions": []interface{}{
									map[string]interface{}{
										"slots": map[string]interface{}{
											"main": map[string]interface{}{
												"*": "This page contains publc text that should be flagged.",
											},
										},
									},
								},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
		}

		// Default empty response
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":{}}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.CheckTerminology(ctx, CheckTerminologyArgs{
		Pages: []string{"Test Page"},
	})

	if err != nil {
		t.Fatalf("CheckTerminology failed: %v", err)
	}

	if result.TermsLoaded != 1 {
		t.Errorf("TermsLoaded = %d, want 1", result.TermsLoaded)
	}
	if result.PagesChecked != 1 {
		t.Errorf("PagesChecked = %d, want 1", result.PagesChecked)
	}
	if result.IssuesFound != 1 {
		t.Errorf("IssuesFound = %d, want 1", result.IssuesFound)
	}
}

func TestCheckTerminology_EmptyGlossary(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		if action == "query" {
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"1": map[string]interface{}{
							"pageid": float64(1),
							"title":  "Brand Terminology Glossary",
							"revisions": []interface{}{
								map[string]interface{}{
									"slots": map[string]interface{}{
										"main": map[string]interface{}{
											"*": "Just some text, no tables",
										},
									},
								},
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.CheckTerminology(ctx, CheckTerminologyArgs{
		Pages: []string{"Test Page"},
	})

	if err == nil {
		t.Fatal("Expected error for empty glossary")
	}
}

func TestCheckTranslations_NoLanguages(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.CheckTranslations(ctx, CheckTranslationsArgs{
		BasePages: []string{"Test"},
		Languages: []string{}, // Empty
	})

	if err == nil {
		t.Fatal("Expected error for missing languages")
	}
}

func TestCheckTranslations_NoPagesOrCategory(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.CheckTranslations(ctx, CheckTranslationsArgs{
		Languages: []string{"en", "de"},
		// No pages or category
	})

	if err == nil {
		t.Fatal("Expected error for missing pages/category")
	}
}

func TestCheckTranslations_InvalidPattern(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.CheckTranslations(ctx, CheckTranslationsArgs{
		BasePages: []string{"Test"},
		Languages: []string{"en"},
		Pattern:   "invalid_pattern",
	})

	if err == nil {
		t.Fatal("Expected error for invalid pattern")
	}
}

func TestCheckTranslations_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		if action == "query" {
			titles := r.FormValue("titles")
			prop := r.FormValue("prop")

			// Simulating page info requests
			if prop == "info|categories|links" || prop == "info" || titles != "" {
				pageExists := titles == "Test/en" // Only English exists
				if pageExists {
					response := map[string]interface{}{
						"query": map[string]interface{}{
							"pages": map[string]interface{}{
								"1": map[string]interface{}{
									"pageid":    float64(1),
									"ns":        float64(0),
									"title":     titles,
									"length":    float64(100),
									"lastrevid": float64(123),
								},
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
					return
				} else {
					response := map[string]interface{}{
						"query": map[string]interface{}{
							"pages": map[string]interface{}{
								"-1": map[string]interface{}{
									"ns":      float64(0),
									"title":   titles,
									"missing": "",
								},
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
					return
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":{}}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.CheckTranslations(ctx, CheckTranslationsArgs{
		BasePages: []string{"Test"},
		Languages: []string{"en", "de"},
		Pattern:   "subpage",
	})

	if err != nil {
		t.Fatalf("CheckTranslations failed: %v", err)
	}

	if result.PagesChecked != 1 {
		t.Errorf("PagesChecked = %d, want 1", result.PagesChecked)
	}
	if result.Pattern != "subpage" {
		t.Errorf("Pattern = %q, want %q", result.Pattern, "subpage")
	}
	if result.MissingCount != 1 {
		t.Errorf("MissingCount = %d, want 1 (German missing)", result.MissingCount)
	}
}

func TestCheckTranslations_SuffixPattern(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		if action == "query" {
			titles := r.FormValue("titles")
			// All pages exist
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"1": map[string]interface{}{
							"pageid":    float64(1),
							"ns":        float64(0),
							"title":     titles,
							"length":    float64(100),
							"lastrevid": float64(123),
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.CheckTranslations(ctx, CheckTranslationsArgs{
		BasePages: []string{"Test"},
		Languages: []string{"en"},
		Pattern:   "suffix",
	})

	if err != nil {
		t.Fatalf("CheckTranslations failed: %v", err)
	}

	if result.Pattern != "suffix" {
		t.Errorf("Pattern = %q, want %q", result.Pattern, "suffix")
	}
	if len(result.Pages) != 1 {
		t.Errorf("Pages count = %d, want 1", len(result.Pages))
	}
}

func TestCheckTranslations_PrefixPattern(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		if action == "query" {
			titles := r.FormValue("titles")
			// All pages exist
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"1": map[string]interface{}{
							"pageid":    float64(1),
							"ns":        float64(0),
							"title":     titles,
							"length":    float64(100),
							"lastrevid": float64(123),
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.CheckTranslations(ctx, CheckTranslationsArgs{
		BasePages: []string{"Test"},
		Languages: []string{"en"},
		Pattern:   "prefix",
	})

	if err != nil {
		t.Fatalf("CheckTranslations failed: %v", err)
	}

	if result.Pattern != "prefix" {
		t.Errorf("Pattern = %q, want %q", result.Pattern, "prefix")
	}
}

func TestExtractExternalURLs(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		limit     int
		wantCount int
		wantURLs  []string
	}{
		{
			name:      "extracts https URLs",
			content:   "Visit https://example.com for more info",
			limit:     10,
			wantCount: 1,
			wantURLs:  []string{"https://example.com"},
		},
		{
			name:      "extracts http URLs",
			content:   "See http://test.org/page for details",
			limit:     10,
			wantCount: 1,
			wantURLs:  []string{"http://test.org/page"},
		},
		{
			name:      "extracts multiple URLs",
			content:   "Check https://a.com and http://b.com for info",
			limit:     10,
			wantCount: 2,
		},
		{
			name:      "respects limit",
			content:   "URLs: https://a.com https://b.com https://c.com",
			limit:     2,
			wantCount: 2,
		},
		{
			name:      "deduplicates URLs",
			content:   "Visit https://same.com and https://same.com again",
			limit:     10,
			wantCount: 1,
		},
		{
			name:      "removes trailing punctuation",
			content:   "See https://example.com. Also https://test.org!",
			limit:     10,
			wantCount: 2,
			wantURLs:  []string{"https://example.com", "https://test.org"},
		},
		{
			name:      "handles wiki external link syntax",
			content:   "[https://example.com/path Example Site]",
			limit:     10,
			wantCount: 1,
		},
		{
			name:      "handles empty content",
			content:   "",
			limit:     10,
			wantCount: 0,
		},
		{
			name:      "handles content without URLs",
			content:   "Just plain text without any links",
			limit:     10,
			wantCount: 0,
		},
		{
			name:      "handles zero limit",
			content:   "https://example.com",
			limit:     0,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urls := extractExternalURLs(tt.content, tt.limit)
			if len(urls) != tt.wantCount {
				t.Errorf("extractExternalURLs() returned %d URLs, want %d: %v", len(urls), tt.wantCount, urls)
			}
			if tt.wantURLs != nil {
				for i, want := range tt.wantURLs {
					if i >= len(urls) {
						t.Errorf("missing URL[%d]: want %q", i, want)
						continue
					}
					if urls[i] != want {
						t.Errorf("URL[%d] = %q, want %q", i, urls[i], want)
					}
				}
			}
		})
	}
}

func TestHealthAudit_AnonymousAccess(t *testing.T) {
	// HealthAudit without credentials proceeds (anonymous access).
	// It does not error at the credentials check; it proceeds to
	// API calls and handles failures internally.
	config := &Config{
		BaseURL: "https://test.wiki.com/api.php",
		Timeout: 2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(config, logger)
	defer client.Close()

	ctx := context.Background()
	result, err := client.HealthAudit(ctx, WikiHealthAuditArgs{})

	// HealthAudit is resilient: it may return a result with failures
	// rather than an error. Either way, it should NOT fail with "no credentials".
	if err != nil && strings.Contains(err.Error(), "no credentials") {
		t.Fatalf("Should not get credentials error for anonymous access, got: %v", err)
	}
	_ = result
}

func TestCheckTerminology_WithCategory(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	// Test with category - exercises that code path
	_, err := client.CheckTerminology(context.Background(), CheckTerminologyArgs{
		Category: "Test Category",
	})
	// Will fail due to no credentials, but exercises path
	_ = err
}

func TestCheckTerminology_WithLimit(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.CheckTerminology(context.Background(), CheckTerminologyArgs{
		Pages: []string{"Page1", "Page2"},
		Limit: 5,
	})
	_ = err
}

func TestCheckTranslations_WithOptions(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.CheckTranslations(context.Background(), CheckTranslationsArgs{
		Languages: []string{"en", "no", "de"},
		Category:  "Test",
		Pattern:   "subpages",
		Limit:     10,
	})
	_ = err
}
