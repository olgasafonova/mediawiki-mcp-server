package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"plain error", errors.New("boom"), exitDefault},
		{"usageErr", usageErr(errors.New("bad flag")), exitUsage},
		{"notFoundErr", notFoundErr(errors.New("missing")), exitNotFound},
		{"authErr", authErr(errors.New("denied")), exitAuth},
		{"apiErr", apiErr(errors.New("server")), exitAPI},
		{"rateLimitErr", rateLimitErr(errors.New("slow down")), exitRateLimit},
		{"configErr", configErr(errors.New("bad config")), exitConfig},
		{"APIError 404", &wiki.APIError{StatusCode: 404}, exitNotFound},
		{"APIError 401", &wiki.APIError{StatusCode: 401}, exitAuth},
		{"APIError 403", &wiki.APIError{StatusCode: 403}, exitAuth},
		{"APIError 429", &wiki.APIError{StatusCode: 429}, exitRateLimit},
		{"APIError 500", &wiki.APIError{StatusCode: 500}, exitAPI},
		{"APIError 418", &wiki.APIError{StatusCode: 418}, exitAPI},
		{"wrapped APIError", fmt.Errorf("during edit: %w", &wiki.APIError{StatusCode: 404}), exitNotFound},
		{"cliError wins over APIError", usageErr(&wiki.APIError{StatusCode: 404}), exitUsage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExitCode(tc.err)
			if got != tc.want {
				t.Errorf("ExitCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

func TestExitCodeConstantsNoCollision(t *testing.T) {
	codes := map[int]string{
		exitDefault:      "default",
		exitUsage:        "usage",
		exitNotFound:     "notFound",
		exitLintFindings: "lintFindings",
		exitAPI:          "api",
		exitAuth:         "auth",
		exitRateLimit:    "rateLimit",
		exitConfig:       "config",
	}
	if len(codes) != 8 {
		t.Fatalf("exit code collision: %d unique codes for 8 constants", len(codes))
	}
}
