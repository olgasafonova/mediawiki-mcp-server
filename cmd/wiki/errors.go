package main

import (
	"errors"
	"net/http"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

// Typed exit codes. Adapted from the CLI Printing Press canonical map
// (usageErr=2, notFoundErr=3, authErr=4, apiErr=5, rateLimitErr=7, configErr=10),
// with one deviation: code 4 is reserved for `wiki lint` findings (existing
// public API documented in CHANGELOG.md and README.md), so authErr moves to 6.
const (
	exitDefault      = 1
	exitUsage        = 2
	exitNotFound     = 3
	exitLintFindings = 4 // reserved; wiki lint sets this directly
	exitAPI          = 5
	exitAuth         = 6
	exitRateLimit    = 7
	exitConfig       = 10
)

// cliError carries an exit code alongside the underlying error so that main()
// can translate a returned error into the right shell exit code without each
// command having to call os.Exit itself.
type cliError struct {
	code int
	err  error
}

func (e *cliError) Error() string { return e.err.Error() }
func (e *cliError) Unwrap() error { return e.err }

func usageErr(err error) error     { return &cliError{code: exitUsage, err: err} }
func notFoundErr(err error) error  { return &cliError{code: exitNotFound, err: err} }
func authErr(err error) error      { return &cliError{code: exitAuth, err: err} }
func apiErr(err error) error       { return &cliError{code: exitAPI, err: err} }
func rateLimitErr(err error) error { return &cliError{code: exitRateLimit, err: err} }
func configErr(err error) error    { return &cliError{code: exitConfig, err: err} }

// ExitCode resolves an error to a shell exit code. The resolution order is:
//  1. Explicit cliError wrapping → that code.
//  2. wiki.APIError → classified by HTTP status (404→notFound, 401/403→auth,
//     429→rateLimit, other 4xx/5xx→api).
//  3. Anything else → exitDefault.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code
	}
	var apiE *wiki.APIError
	if errors.As(err, &apiE) {
		switch apiE.StatusCode {
		case http.StatusNotFound:
			return exitNotFound
		case http.StatusUnauthorized, http.StatusForbidden:
			return exitAuth
		case http.StatusTooManyRequests:
			return exitRateLimit
		default:
			return exitAPI
		}
	}
	return exitDefault
}
