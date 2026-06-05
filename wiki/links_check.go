package wiki

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

func validateLinkURLForCheck(rawURL string) (LinkCheckResult, bool) {
	r := LinkCheckResult{URL: rawURL}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		r.Status = "invalid_url"
		r.Error = fmt.Sprintf("[%s] Invalid URL format", SSRFCodeInvalidURL)
		r.Broken = true
		return r, false
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		// No hostname to test for SSRF; treat as caller's responsibility but pass.
		return r, true
	}
	isPrivate, ssrfErr := isPrivateHost(hostname)
	if !isPrivate {
		return r, true
	}
	r.Status = "blocked"
	if ssrfErr != nil {
		r.Error = ssrfErr.Error()
	} else {
		r.Error = fmt.Sprintf("[%s] URLs pointing to private/internal networks are not allowed", SSRFCodePrivateIP)
	}
	r.Broken = true
	return r, false
}

// fetchLinkStatus issues a HEAD request, falling back to GET if the server
// rejects HEAD, and writes the resulting status onto r. Marks Broken=true if
// the request fails or returns 4xx/5xx.
func fetchLinkStatus(ctx context.Context, rawURL string, timeout time.Duration, r *LinkCheckResult) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	makeRequest := func(method string) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(reqCtx, method, rawURL, nil)
		req.Header.Set("User-Agent", "MediaWiki-MCP-LinkChecker/1.0")
		return linkCheckClient.Do(req) // #nosec G704 -- link checker intentionally fetches external URLs
	}

	resp, err := makeRequest("HEAD")
	if err != nil {
		resp, err = makeRequest("GET")
	}
	if err != nil {
		r.Status = "error"
		r.Error = err.Error()
		r.Broken = true
		return
	}
	_ = resp.Body.Close()
	r.StatusCode = resp.StatusCode
	r.Status = resp.Status
	if resp.StatusCode >= 400 {
		r.Broken = true
	}
}

// checkSingleLink performs validation + status fetch for a single URL.
func checkSingleLink(ctx context.Context, rawURL string, timeout time.Duration) LinkCheckResult {
	r, ok := validateLinkURLForCheck(rawURL)
	if !ok {
		return r
	}
	fetchLinkStatus(ctx, rawURL, timeout, &r)
	return r
}

// resolveLinkCheckTimeout clamps the user-supplied timeout to the safe range.
func resolveLinkCheckTimeout(requested int) time.Duration {
	timeout := 10
	if requested > 0 && requested <= 30 {
		timeout = requested
	}
	return time.Duration(timeout) * time.Second
}

func (c *Client) CheckLinks(ctx context.Context, args CheckLinksArgs) (CheckLinksResult, error) {
	if len(args.URLs) == 0 {
		return CheckLinksResult{}, fmt.Errorf("at least one URL is required")
	}

	const maxURLs = 20
	if len(args.URLs) > maxURLs {
		args.URLs = args.URLs[:maxURLs]
	}
	requestTimeout := resolveLinkCheckTimeout(args.Timeout)

	result := CheckLinksResult{
		Results:    make([]LinkCheckResult, 0, len(args.URLs)),
		TotalLinks: len(args.URLs),
	}

	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, linkURL := range args.URLs {
		wg.Add(1)
		go func(rawURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			linkResult := checkSingleLink(ctx, rawURL, requestTimeout)
			mu.Lock()
			defer mu.Unlock()
			result.Results = append(result.Results, linkResult)
			if linkResult.Broken {
				result.BrokenCount++
			} else {
				result.ValidCount++
			}
		}(linkURL)
	}
	wg.Wait()
	return result, nil
}

// GetBacklinks returns pages that link to the specified page ("What links here")
