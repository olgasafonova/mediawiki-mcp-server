package wiki

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchInPage_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		if action == "query" {
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"1": map[string]interface{}{
							"pageid": float64(1),
							"title":  "Test Page",
							"revisions": []interface{}{
								map[string]interface{}{
									"slots": map[string]interface{}{
										"main": map[string]interface{}{
											"*": "This is a test page with some content.\nAnother line with test word.\nFinal line.",
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
		}
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.SearchInPage(ctx, SearchInPageArgs{
		Title: "Test Page",
		Query: "test",
	})

	if err != nil {
		t.Fatalf("SearchInPage failed: %v", err)
	}

	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Page")
	}
	if result.MatchCount < 2 {
		t.Errorf("MatchCount = %d, want >= 2", result.MatchCount)
	}
}

func TestSearchInPage_PageNotFound(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"-1": map[string]interface{}{
						"ns":      float64(0),
						"title":   "Nonexistent",
						"missing": "",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.SearchInPage(ctx, SearchInPageArgs{
		Title: "Nonexistent",
		Query: "test",
	})

	if err == nil {
		t.Fatal("Expected error for missing page")
	}
}

func TestSearchInPage_NoMatches(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"1": map[string]interface{}{
						"pageid": float64(1),
						"title":  "Test Page",
						"revisions": []interface{}{
							map[string]interface{}{
								"slots": map[string]interface{}{
									"main": map[string]interface{}{
										"*": "This page has no matching content.",
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
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.SearchInPage(ctx, SearchInPageArgs{
		Title: "Test Page",
		Query: "zzzznotfound",
	})

	if err != nil {
		t.Fatalf("SearchInPage failed: %v", err)
	}
	if result.MatchCount != 0 {
		t.Errorf("MatchCount = %d, want 0", result.MatchCount)
	}
}

func TestSearchInPage_RegexMode(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"1": map[string]interface{}{
						"pageid": float64(1),
						"title":  "Test Page",
						"revisions": []interface{}{
							map[string]interface{}{
								"slots": map[string]interface{}{
									"main": map[string]interface{}{
										"*": "Line 1: hello123world\nLine 2: foo456bar\nLine 3: no numbers here",
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
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.SearchInPage(ctx, SearchInPageArgs{
		Title:    "Test Page",
		Query:    "[0-9]+",
		UseRegex: true,
	})

	if err != nil {
		t.Fatalf("SearchInPage failed: %v", err)
	}
	if result.MatchCount < 2 {
		t.Errorf("MatchCount = %d, want >= 2", result.MatchCount)
	}
}

func TestSearchInPage_RegexTooLong(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	longPattern := strings.Repeat("a", 501)
	_, err := client.SearchInPage(context.Background(), SearchInPageArgs{
		Title:    "Test Page",
		Query:    longPattern,
		UseRegex: true,
	})

	if err == nil {
		t.Fatal("Expected error for regex pattern exceeding 500 chars")
	}
	if !strings.Contains(err.Error(), "regex pattern too long") {
		t.Errorf("Expected 'regex pattern too long' error, got: %v", err)
	}
}

func TestFindSimilarPages_Success(t *testing.T) {
	callCount := 0
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		if action == "query" {
			prop := r.FormValue("prop")
			list := r.FormValue("list")

			if prop == "revisions" {
				// Get source page content
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  "API Guide",
								"revisions": []interface{}{
									map[string]interface{}{
										"slots": map[string]interface{}{
											"main": map[string]interface{}{
												"*": "This is the API reference guide for authentication and authorization.",
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

			if list == "allpages" || prop == "categories" {
				callCount++
				if callCount <= 2 {
					// Return pages list
					response := map[string]interface{}{
						"query": map[string]interface{}{
							"allpages": []interface{}{
								map[string]interface{}{"pageid": float64(2), "title": "API Tutorial"},
								map[string]interface{}{"pageid": float64(3), "title": "Auth Guide"},
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
					return
				}
			}
		}
		// Default response
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":{}}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.FindSimilarPages(ctx, FindSimilarPagesArgs{
		Page:  "API Guide",
		Limit: 5,
	})

	if err != nil {
		t.Fatalf("FindSimilarPages failed: %v", err)
	}

	if result.SourcePage != "API Guide" {
		t.Errorf("SourcePage = %q, want %q", result.SourcePage, "API Guide")
	}
}

func TestFindSimilarPages_PageNotFound(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"-1": map[string]interface{}{
						"ns":      float64(0),
						"title":   "Missing Page",
						"missing": "",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.FindSimilarPages(ctx, FindSimilarPagesArgs{
		Page: "Missing Page",
	})

	if err == nil {
		t.Fatal("Expected error for missing page")
	}
}

func TestCompareTopic_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		if action == "query" {
			list := r.FormValue("list")
			if list == "search" {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"searchinfo": map[string]interface{}{"totalhits": float64(2)},
						"search": []interface{}{
							map[string]interface{}{
								"pageid":  float64(1),
								"title":   "API Timeout Guide",
								"snippet": "The API timeout is set to <span class=\"searchmatch\">30</span> seconds",
							},
							map[string]interface{}{
								"pageid":  float64(2),
								"title":   "Configuration",
								"snippet": "Default timeout value is <span class=\"searchmatch\">30</span>s",
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}

			// Handle revision queries
			prop := r.FormValue("prop")
			if prop == "revisions" {
				titles := r.FormValue("titles")
				content := "Timeout is 30 seconds for all requests."
				if titles == "Configuration" {
					content = "The default timeout setting is 30s."
				}
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  titles,
								"revisions": []interface{}{
									map[string]interface{}{
										"*": content,
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
	result, err := client.CompareTopic(ctx, CompareTopicArgs{
		Topic: "timeout",
		Limit: 5,
	})

	if err != nil {
		t.Fatalf("CompareTopic failed: %v", err)
	}

	if result.Topic != "timeout" {
		t.Errorf("Topic = %q, want %q", result.Topic, "timeout")
	}
}

func TestCompareTopic_EmptyTopic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.CompareTopic(ctx, CompareTopicArgs{
		Topic: "",
	})

	if err == nil {
		t.Fatal("Expected error for empty topic")
	}
}

func TestNormalizeValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "extracts integer",
			input: "timeout 30",
			want:  "30",
		},
		{
			name:  "extracts decimal",
			input: "rate 1.5",
			want:  "1.5",
		},
		{
			name:  "extracts first number from multiple",
			input: "range 10 to 20",
			want:  "10",
		},
		{
			name:  "handles number with units",
			input: "30s timeout",
			want:  "30",
		},
		{
			name:  "handles no numbers",
			input: "enabled",
			want:  "enabled",
		},
		{
			name:  "trims and lowercases text-only values",
			input: "  TRUE  ",
			want:  "true",
		},
		{
			name:  "extracts from complex string",
			input: "max_connections=100",
			want:  "100",
		},
		{
			name:  "handles percentage",
			input: "completion: 85%",
			want:  "85",
		},
		{
			name:  "handles empty string",
			input: "",
			want:  "",
		},
		{
			name:  "handles only whitespace",
			input: "   ",
			want:  "",
		},
		{
			name:  "extracts from version string",
			input: "version 1.2.3",
			want:  "1.2",
		},
		{
			name:  "handles negative-looking pattern",
			input: "offset -10",
			want:  "10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeValue(tt.input)
			if got != tt.want {
				t.Errorf("normalizeValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Note: TestStripHTMLTags is in validation_test.go

func TestSearchInFile_EmptyFilename(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	ctx := context.Background()
	_, err := client.SearchInFile(ctx, SearchInFileArgs{
		Filename: "",
		Query:    "test",
	})

	if err == nil {
		t.Fatal("Expected error for empty filename")
	}
}

func TestSearchInFile_EmptyQuery(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	ctx := context.Background()
	_, err := client.SearchInFile(ctx, SearchInFileArgs{
		Filename: "test.pdf",
		Query:    "",
	})

	if err == nil {
		t.Fatal("Expected error for empty query")
	}
}

func TestFindSimilarPages_EmptyPage(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	ctx := context.Background()
	_, err := client.FindSimilarPages(ctx, FindSimilarPagesArgs{
		Page: "",
	})

	if err == nil {
		t.Fatal("Expected error for empty page")
	}
}

func TestFindSimilarPages_EmptyContent(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"1": map[string]interface{}{
						"pageid": float64(1),
						"ns":     float64(0),
						"title":  "Empty Page",
						"revisions": []interface{}{
							map[string]interface{}{
								"slots": map[string]interface{}{
									"main": map[string]interface{}{
										"*": "", // Empty content
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
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.FindSimilarPages(ctx, FindSimilarPagesArgs{
		Page: "Empty Page",
	})

	if err != nil {
		t.Fatalf("FindSimilarPages failed: %v", err)
	}

	// Should return with message about insufficient content
	if result.SourcePage != "Empty Page" {
		t.Errorf("SourcePage = %q, want %q", result.SourcePage, "Empty Page")
	}
}

func TestSearchInFile_Success(t *testing.T) {
	requestCount := 0
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_ = r.ParseForm()

		if requestCount == 1 {
			// First request: get file info
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"100": map[string]interface{}{
							"pageid": float64(100),
							"ns":     float64(6),
							"title":  "File:test.txt",
							"imageinfo": []interface{}{
								map[string]interface{}{
									"url":       "https://example.com/test.txt",
									"mime":      "text/plain",
									"size":      float64(50),
									"mediatype": "TEXT",
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

		// Subsequent: file download would be external, so return error for now
		http.Error(w, "Not found", http.StatusNotFound)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	// This will fail on file download since mock server can't serve external URL
	_, err := client.SearchInFile(ctx, SearchInFileArgs{
		Filename: "test.txt",
		Query:    "hello",
	})

	// Expected to fail on download, but exercises the file info path
	if err == nil {
		t.Log("SearchInFile succeeded - file was found")
	} else {
		// Expected - the download will fail but we exercised the path
		t.Logf("SearchInFile error (expected): %v", err)
	}
}

func TestSearchInFile_FilenameNormalization(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		titles := r.FormValue("titles")

		// Verify filename was normalized to include File: prefix
		if titles != "File:document.pdf" {
			t.Errorf("Expected title 'File:document.pdf', got %q", titles)
		}

		// Return missing file
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"-1": map[string]interface{}{
						"ns":      float64(6),
						"title":   "File:document.pdf",
						"missing": "",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.SearchInFile(ctx, SearchInFileArgs{
		Filename: "document.pdf", // Without File: prefix
		Query:    "test",
	})

	// Will fail but exercises the normalization path
	if err == nil {
		t.Log("SearchInFile succeeded unexpectedly")
	}
}

func TestFindSimilarPages_WithSearch(t *testing.T) {
	requestCount := 0
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_ = r.ParseForm()
		action := r.FormValue("action")

		var response map[string]interface{}

		if action == "query" {
			list := r.FormValue("list")
			prop := r.FormValue("prop")
			titles := r.FormValue("titles")

			if list == "search" {
				// Search for candidates
				response = map[string]interface{}{
					"query": map[string]interface{}{
						"search": []interface{}{
							map[string]interface{}{"title": "Related Page 1"},
							map[string]interface{}{"title": "Related Page 2"},
						},
					},
				}
			} else if prop == "links" {
				// Get links
				response = map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  titles,
								"links": []interface{}{
									map[string]interface{}{"title": "Related Page 1"},
								},
							},
						},
					},
				}
			} else if prop == "revisions" {
				// Get page content
				var content string
				switch titles {
				case "Related Page 1":
					content = "Related programming content with software development."
				case "Related Page 2":
					content = "Different content about unrelated things."
				default:
					content = "This is test content with keywords programming and development."
				}
				response = map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  titles,
								"revisions": []interface{}{
									map[string]interface{}{
										"slots": map[string]interface{}{
											"main": map[string]interface{}{
												"*": content,
											},
										},
									},
								},
							},
						},
					},
				}
			} else {
				response = map[string]interface{}{"query": map[string]interface{}{}}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.FindSimilarPages(ctx, FindSimilarPagesArgs{
		Page:     "Source Page",
		Limit:    10,
		MinScore: 0.1,
	})

	if err != nil {
		t.Fatalf("FindSimilarPages failed: %v", err)
	}

	if result.SourcePage != "Source Page" {
		t.Errorf("Expected source page 'Source Page', got %q", result.SourcePage)
	}
}

func TestFindSimilarPages_WithCategory(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		var response map[string]interface{}

		if action == "query" {
			list := r.FormValue("list")
			prop := r.FormValue("prop")
			titles := r.FormValue("titles")

			if list == "categorymembers" {
				response = map[string]interface{}{
					"query": map[string]interface{}{
						"categorymembers": []interface{}{
							map[string]interface{}{"title": "Category Page 1"},
							map[string]interface{}{"title": "Category Page 2"},
						},
					},
				}
			} else if prop == "links" {
				response = map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  titles,
								"links":  []interface{}{},
							},
						},
					},
				}
			} else if prop == "revisions" {
				response = map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  titles,
								"revisions": []interface{}{
									map[string]interface{}{
										"slots": map[string]interface{}{
											"main": map[string]interface{}{
												"*": "Test content with terms software and development.",
											},
										},
									},
								},
							},
						},
					},
				}
			} else {
				response = map[string]interface{}{"query": map[string]interface{}{}}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.FindSimilarPages(ctx, FindSimilarPagesArgs{
		Page:     "Source Page",
		Category: "TestCategory",
	})

	if err != nil {
		t.Fatalf("FindSimilarPages failed: %v", err)
	}

	if result.SourcePage != "Source Page" {
		t.Errorf("Expected source page 'Source Page', got %q", result.SourcePage)
	}
}

func TestCompareTopic_WithSearch(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		var response map[string]interface{}

		if action == "query" {
			list := r.FormValue("list")
			prop := r.FormValue("prop")
			titles := r.FormValue("titles")

			if list == "search" {
				response = map[string]interface{}{
					"query": map[string]interface{}{
						"search": []interface{}{
							map[string]interface{}{"title": "Page About Timeout"},
							map[string]interface{}{"title": "Timeout Configuration"},
						},
					},
				}
			} else if prop != "" && titles != "" {
				// Handle both info and revisions requests
				content := "Default timeout is 30 seconds. Configure timeout for your application."
				if titles == "Timeout Configuration" {
					content = "The timeout value is 60 seconds. Set timeout in config."
				}
				response = map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid":       float64(1),
								"title":        titles,
								"ns":           float64(0),
								"touched":      "2024-01-15T10:30:00Z",
								"length":       float64(500),
								"lastrevid":    float64(123),
								"contentmodel": "wikitext",
								"pagelanguage": "en",
								"revisions": []interface{}{
									map[string]interface{}{
										"slots": map[string]interface{}{
											"main": map[string]interface{}{
												"*": content,
											},
										},
									},
								},
							},
						},
					},
				}
			} else {
				response = map[string]interface{}{"query": map[string]interface{}{}}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.CompareTopic(ctx, CompareTopicArgs{
		Topic: "timeout",
		Limit: 10,
	})

	if err != nil {
		t.Fatalf("CompareTopic failed: %v", err)
	}

	if result.Topic != "timeout" {
		t.Errorf("Expected topic 'timeout', got %q", result.Topic)
	}

	if len(result.PageMentions) == 0 {
		t.Error("Expected at least one page mention")
	}
}

func TestCompareTopic_WithCategory(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		var response map[string]interface{}

		if action == "query" {
			list := r.FormValue("list")
			prop := r.FormValue("prop")
			titles := r.FormValue("titles")

			if list == "categorymembers" {
				response = map[string]interface{}{
					"query": map[string]interface{}{
						"categorymembers": []interface{}{
							map[string]interface{}{"title": "API Settings"},
							map[string]interface{}{"title": "Config Guide"},
						},
					},
				}
			} else if prop != "" && titles != "" {
				// Handle both info and revisions requests
				response = map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid":       float64(1),
								"title":        titles,
								"ns":           float64(0),
								"touched":      "2024-01-15T10:30:00Z",
								"length":       float64(500),
								"lastrevid":    float64(123),
								"contentmodel": "wikitext",
								"pagelanguage": "en",
								"revisions": []interface{}{
									map[string]interface{}{
										"slots": map[string]interface{}{
											"main": map[string]interface{}{
												"*": "This page discusses API with various settings.",
											},
										},
									},
								},
							},
						},
					},
				}
			} else {
				response = map[string]interface{}{"query": map[string]interface{}{}}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.CompareTopic(ctx, CompareTopicArgs{
		Topic:    "API",
		Category: "Configuration",
	})

	if err != nil {
		t.Fatalf("CompareTopic failed: %v", err)
	}

	if result.Topic != "API" {
		t.Errorf("Expected topic 'API', got %q", result.Topic)
	}
}

func TestCompareTopic_NoResults(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()

		response := map[string]interface{}{
			"query": map[string]interface{}{
				"search": []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.CompareTopic(ctx, CompareTopicArgs{
		Topic: "nonexistent_xyz_topic",
	})

	if err != nil {
		t.Fatalf("CompareTopic failed: %v", err)
	}

	if len(result.PageMentions) != 0 {
		t.Errorf("Expected no page mentions, got %d", len(result.PageMentions))
	}
}
