package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

// cwCaptchaCmd builds the minimal command the CAPTCHA helpers read from: a
// --json flag for isJSON and a context for the edit request.
func cwCaptchaCmd(jsonMode bool) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", jsonMode, "")
	cmd.SetContext(context.Background())
	return cmd
}

// cwStubCaptchaPrompt scripts the CAPTCHA prompt IO for one test. A nil in
// models "no interactive input available", which is what a piped invocation
// with no /dev/tty gets.
func cwStubCaptchaPrompt(t *testing.T, in io.Reader, out io.Writer) {
	t.Helper()
	original := captchaPromptIOFunc
	captchaPromptIOFunc = func() (io.Reader, io.Writer, func()) { return in, out, func() {} }
	t.Cleanup(func() { captchaPromptIOFunc = original })
}

// cwCaptchaWiki is a minimal MediaWiki API stand-in for the CAPTCHA-retry
// path: it reports an existing session, hands out a CSRF token, and answers
// one edit request. Edit form values are recorded so a test can assert the
// CAPTCHA id and word were forwarded.
type cwCaptchaWiki struct {
	mu       sync.Mutex
	editForm url.Values
	editResp map[string]any
}

func (s *cwCaptchaWiki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)

	action, meta := r.FormValue("action"), r.FormValue("meta")
	switch {
	case action == "query" && meta == "userinfo":
		_ = enc.Encode(map[string]any{"query": map[string]any{
			"userinfo": map[string]any{"id": float64(1), "name": "TestUser"},
		}})
	case action == "query" && meta == "tokens":
		_ = enc.Encode(map[string]any{"query": map[string]any{
			"tokens": map[string]any{"csrftoken": "test-csrf-token"},
		}})
	case action == "edit":
		s.mu.Lock()
		s.editForm = r.Form
		s.mu.Unlock()
		_ = enc.Encode(map[string]any{"edit": s.editResp})
	default:
		// siteinfo and anything else: an empty payload is enough for the
		// page-URL lookup to fall back to its api.php-relative form.
		_ = enc.Encode(map[string]any{})
	}
}

func (s *cwCaptchaWiki) form() url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.editForm
}

// cwCaptchaSession wires a session to a stub wiki so retry submissions hit a
// local server instead of a real wiki.
func cwCaptchaSession(t *testing.T, wikiStub *cwCaptchaWiki, req editRequest) editSession {
	t.Helper()
	ts := httptest.NewServer(wikiStub)
	t.Cleanup(ts.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	client := wiki.NewClient(&wiki.Config{
		BaseURL:    ts.URL,
		Username:   "TestUser",
		Password:   "TestPass",
		Timeout:    5 * time.Second,
		MaxRetries: 1,
		UserAgent:  "wiki-cli-test/1.0",
	}, logger)
	t.Cleanup(client.Close)

	return editSession{client: client, req: req}
}

// cwCaptureStderr runs fn with os.Stderr redirected and returns what it wrote.
func cwCaptureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = w

	captured := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		captured <- string(b)
	}()

	fn()

	_ = w.Close()
	os.Stderr = original
	return <-captured
}

// TestRetryLoopReturnsWithoutPrompting covers the two states that must never
// reach a prompt: a successful edit, and a failure the wiki didn't attach a
// CAPTCHA to.
func TestRetryLoopReturnsWithoutPrompting(t *testing.T) {
	cases := []struct {
		name   string
		result wiki.EditResult
	}{
		{"successful edit", wiki.EditResult{Success: true, Title: "Page", RevisionID: 9}},
		{"failure without captcha", wiki.EditResult{Success: false, Title: "Page", Message: "protected page"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prompted := false
			original := captchaPromptIOFunc
			captchaPromptIOFunc = func() (io.Reader, io.Writer, func()) {
				prompted = true
				return nil, nil, func() {}
			}
			t.Cleanup(func() { captchaPromptIOFunc = original })

			session := editSession{req: editRequest{title: "Page"}}
			got := session.retryLoop(cwCaptchaCmd(false), tc.result)

			if prompted {
				t.Error("retryLoop prompted for a CAPTCHA that was never requested")
			}
			if got != tc.result {
				t.Errorf("retryLoop = %+v, want %+v unchanged", got, tc.result)
			}
		})
	}
}

// TestRetryLoopSkipsPromptInJSONMode locks in the --json contract: prompting
// would corrupt the JSON stream, so the CAPTCHA details are handed back for
// the caller to print instead.
func TestRetryLoopSkipsPromptInJSONMode(t *testing.T) {
	prompted := false
	original := captchaPromptIOFunc
	captchaPromptIOFunc = func() (io.Reader, io.Writer, func()) {
		prompted = true
		return nil, nil, func() {}
	}
	t.Cleanup(func() { captchaPromptIOFunc = original })

	result := wiki.EditResult{
		Success:         false,
		Title:           "Page",
		CaptchaType:     "math",
		CaptchaID:       "captcha-1",
		CaptchaQuestion: "2+2",
	}

	session := editSession{req: editRequest{title: "Page"}}
	got := session.retryLoop(cwCaptchaCmd(true), result)

	if prompted {
		t.Error("retryLoop prompted under --json")
	}
	if got.CaptchaType != "math" || got.CaptchaID != "captcha-1" {
		t.Errorf("retryLoop dropped CAPTCHA details under --json: %+v", got)
	}
}

// TestRetryLoopTerminatesWithoutInteractiveInput is the anti-hang regression:
// with no prompt available the helper must clear CaptchaType so the loop
// condition goes false instead of spinning forever.
func TestRetryLoopTerminatesWithoutInteractiveInput(t *testing.T) {
	cwStubCaptchaPrompt(t, nil, nil)

	result := wiki.EditResult{Success: false, Title: "Page", CaptchaType: "math", CaptchaID: "captcha-1"}
	session := editSession{req: editRequest{title: "Page"}}

	var got wiki.EditResult
	stderr := cwCaptureStderr(t, func() {
		got = session.retryLoop(cwCaptchaCmd(false), result)
	})

	if got.CaptchaType != "" {
		t.Errorf("CaptchaType = %q, want cleared so the retry loop can exit", got.CaptchaType)
	}
	if !strings.Contains(stderr, "--json") {
		t.Errorf("stderr hint %q should point the user at --json", stderr)
	}
}

// TestPromptAndRetryCaptchaSubmitsAnswer covers the happy path end to end:
// the answer typed at the prompt is attached to a fresh edit request.
func TestPromptAndRetryCaptchaSubmitsAnswer(t *testing.T) {
	wikiStub := &cwCaptchaWiki{editResp: map[string]any{
		"result":   "Success",
		"pageid":   float64(7),
		"title":    "Test Page",
		"newrevid": float64(99),
	}}
	session := cwCaptchaSession(t, wikiStub, editRequest{
		title:   "Test Page",
		content: "= Hi =",
		summary: "captcha retry",
	})

	var prompt bytes.Buffer
	cwStubCaptchaPrompt(t, strings.NewReader("42\n"), &prompt)

	got := session.promptAndRetryCaptcha(cwCaptchaCmd(false), wiki.EditResult{
		Success:         false,
		Title:           "Test Page",
		CaptchaType:     "math",
		CaptchaID:       "captcha-1",
		CaptchaQuestion: "2+2",
	})

	if !got.Success {
		t.Fatalf("retry result = %+v, want success", got)
	}
	if got.RevisionID != 99 {
		t.Errorf("RevisionID = %d, want 99", got.RevisionID)
	}
	if !strings.Contains(prompt.String(), "2+2") {
		t.Errorf("prompt %q should show the CAPTCHA question", prompt.String())
	}

	form := wikiStub.form()
	if got := form.Get("captchaid"); got != "captcha-1" {
		t.Errorf("captchaid = %q, want captcha-1", got)
	}
	if got := form.Get("captchaword"); got != "42" {
		t.Errorf("captchaword = %q, want 42", got)
	}
}

// TestPromptAndRetryCaptchaReprompts covers the blank-line branch: an empty
// answer re-asks rather than giving up or submitting nothing.
func TestPromptAndRetryCaptchaReprompts(t *testing.T) {
	wikiStub := &cwCaptchaWiki{editResp: map[string]any{
		"result":   "Success",
		"title":    "Test Page",
		"newrevid": float64(12),
	}}
	session := cwCaptchaSession(t, wikiStub, editRequest{title: "Test Page", content: "= Hi ="})

	var prompt bytes.Buffer
	cwStubCaptchaPrompt(t, strings.NewReader("\n\n7\n"), &prompt)

	got := session.promptAndRetryCaptcha(cwCaptchaCmd(false), wiki.EditResult{
		Success:     false,
		Title:       "Test Page",
		CaptchaType: "image",
		CaptchaID:   "captcha-2",
	})

	if !got.Success {
		t.Fatalf("retry result = %+v, want success after re-prompting", got)
	}
	if n := strings.Count(prompt.String(), "CAPTCHA required"); n != 3 {
		t.Errorf("prompt shown %d times, want 3 (two blanks then the answer)", n)
	}
	// With no question text the prompt falls back to naming the type.
	if !strings.Contains(prompt.String(), "CAPTCHA type: image") {
		t.Errorf("prompt %q should fall back to the CAPTCHA type", prompt.String())
	}
	if got := wikiStub.form().Get("captchaword"); got != "7" {
		t.Errorf("captchaword = %q, want 7", got)
	}
}

// TestPromptAndRetryCaptchaEOFWithoutAnswer covers a reader that closes before
// an answer arrives: the helper falls back to the non-interactive hint rather
// than submitting an empty CAPTCHA word.
func TestPromptAndRetryCaptchaEOFWithoutAnswer(t *testing.T) {
	wikiStub := &cwCaptchaWiki{}
	session := cwCaptchaSession(t, wikiStub, editRequest{title: "Test Page", content: "= Hi ="})

	var prompt bytes.Buffer
	cwStubCaptchaPrompt(t, strings.NewReader(""), &prompt)

	original := wiki.EditResult{Success: false, Title: "Test Page", CaptchaType: "math", CaptchaID: "captcha-3"}
	var got wiki.EditResult
	stderr := cwCaptureStderr(t, func() {
		got = session.promptAndRetryCaptcha(cwCaptchaCmd(false), original)
	})

	if got.CaptchaType != "" {
		t.Errorf("CaptchaType = %q, want cleared", got.CaptchaType)
	}
	if !strings.Contains(stderr, "no interactive input available") {
		t.Errorf("stderr = %q, want the non-interactive hint", stderr)
	}
	if wikiStub.form() != nil {
		t.Error("an edit was submitted despite no CAPTCHA answer")
	}
}

// TestRetryEditWithCaptchaReportsSubmitError covers the error branch: a failed
// submission is reported as a result rather than surfacing as a Go error, so
// the caller's loop terminates with a message the user can read.
func TestRetryEditWithCaptchaReportsSubmitError(t *testing.T) {
	// Empty content fails client-side validation, so no request is made and
	// the failure is deterministic.
	session := cwCaptchaSession(t, &cwCaptchaWiki{}, editRequest{title: "Test Page"})

	got := session.retryEditWithCaptcha(context.Background(),
		wiki.EditResult{Success: false, Title: "Test Page", CaptchaType: "math", CaptchaID: "captcha-4"},
		"42")

	if got.Success {
		t.Fatalf("retryEditWithCaptcha = %+v, want failure", got)
	}
	if got.Title != "Test Page" {
		t.Errorf("Title = %q, want the requested title preserved", got.Title)
	}
	if !strings.Contains(got.Message, "Edit failed on CAPTCHA retry") {
		t.Errorf("Message = %q, want it to name the CAPTCHA retry", got.Message)
	}
}

// TestCaptchaPromptIOInvariants exercises the real resolver. Which branch it
// takes depends on whether the test host has a controlling terminal, so the
// assertions are the invariants every branch must hold: a usable cleanup, and
// a writer whenever there is a reader.
func TestCaptchaPromptIOInvariants(t *testing.T) {
	in, out, cleanup := captchaPromptIO()
	if cleanup == nil {
		t.Fatal("cleanup is nil; the deferred call in promptAndRetryCaptcha would panic")
	}
	defer cleanup()

	if in != nil && out == nil {
		t.Error("reader without a writer: the prompt would have nowhere to print")
	}
	if in == nil && out != nil {
		t.Error("writer without a reader: the prompt would print with nothing to read")
	}
}

// TestCaptchaPromptIOUsesTerminalDevice covers the preferred branch. A
// writable regular file stands in for /dev/tty so the test holds on hosts
// with no controlling terminal, such as CI.
func TestCaptchaPromptIOUsesTerminalDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake-tty")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("seed device file: %v", err)
	}

	in, out, cleanup := captchaPromptIOFrom(path)
	if in == nil || out == nil {
		t.Fatalf("got reader %v / writer %v, want the opened device for both", in, out)
	}
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	if !inOK || !outOK || inFile != outFile {
		t.Error("reader and writer should be the same handle so the prompt echoes where it reads")
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil")
	}

	// The device is opened read-write, so the prompt can be written to it.
	if _, err := out.Write([]byte("probe")); err != nil {
		t.Errorf("device not writable: %v", err)
	}

	// cleanup must close what it opened; a second close reports an error.
	cleanup()
	if inFile != nil {
		if err := inFile.Close(); err == nil {
			t.Error("cleanup left the device open")
		}
	}
}

// TestCaptchaPromptIOFallsBackToStdin covers the second choice: no terminal
// device, but stdin is itself a terminal.
func TestCaptchaPromptIOFallsBackToStdin(t *testing.T) {
	in, out, cleanup := captchaPromptIOFrom(filepath.Join(t.TempDir(), "no-such-tty"))
	if cleanup == nil {
		t.Fatal("cleanup is nil")
	}
	defer cleanup()

	// Under `go test` stdin is normally /dev/null, which is a character
	// device, so the stdin fallback applies. If the harness ever pipes stdin
	// instead, the correct answer is no prompt at all — assert whichever the
	// mode says, so the test states the rule rather than the environment.
	info, err := os.Stdin.Stat()
	if err != nil {
		t.Skipf("cannot stat stdin: %v", err)
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		if in != os.Stdin {
			t.Errorf("reader = %v, want os.Stdin when stdin is a terminal", in)
		}
		if out != os.Stderr {
			t.Errorf("writer = %v, want os.Stderr so the prompt stays off stdout", out)
		}
		return
	}
	if in != nil {
		t.Errorf("reader = %v, want nil when neither a tty nor a terminal stdin exists", in)
	}
}

// TestCaptchaPromptIOWithoutAnyTerminal covers the last resort: no terminal
// device and a piped stdin. A nil reader is the contract that makes
// promptAndRetryCaptcha print its hint instead of blocking on a read.
func TestCaptchaPromptIOWithoutAnyTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = original
		_ = r.Close()
		_ = w.Close()
	})

	in, out, cleanup := captchaPromptIOFrom(filepath.Join(t.TempDir(), "no-such-tty"))
	if cleanup == nil {
		t.Fatal("cleanup is nil")
	}
	defer cleanup()

	if in != nil || out != nil {
		t.Errorf("got reader %v / writer %v, want both nil for a piped stdin", in, out)
	}
}
