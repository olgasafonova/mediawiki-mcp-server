# MediaWiki MCP Server Constitution

This document holds the governance articles for the MediaWiki MCP Server. These articles are **non-negotiable** and **not subject to per-feature override**. They apply to every commit, pull request, and release regardless of urgency or scope.

This document does not change without an explicit constitutional amendment: a dedicated pull request that modifies only this file, reviewed by the maintainer. A feature pull request that would violate an article does not get an exception; it either changes to comply, or it waits behind an amendment.

**Every article below codifies something the repository already does.** No article invents a new requirement. Each names the file or pattern it is drawn from, and each states honestly whether a linter, a test, or a CI job enforces it, or whether it rests on review alone. An article that claims enforcement it does not have is worse than one that admits it has none, because the false claim stops anyone from adding the missing check.

Written 27-08-2026 against `main` at go-sdk v1.7.0, 43 registered tools (42 declarative specs plus one inline registration, see Article I).

---

## Article I: Tool registration is declarative and single-entry, with one named exception

Adding a tool means adding one `ToolSpec` to a category group in `tools/definitions*.go` (concatenated into `AllTools` by `concatToolSpecs`) and one entry to the `methodRegistrars` map in `tools/handlers.go`. Handlers MUST NOT be registered by hand-written boilerplate: they go through the generic `register[Args, Result]`, which is what attaches panic recovery, OTel tracing, Prometheus metrics, execution logging, and audit events to every tool uniformly. A tool wired around `register` gets none of those and MUST NOT be merged.

Every spec MUST carry a non-empty `Name`, `Method`, and `Description`. Names begin with `mediawiki_`. Names are unique.

The single standing exception: `mediawiki_convert_markdown` is registered inline in `main_httpserver.go` (`registerConverterTool`, line 313) because it wraps the local `converter` package rather than a `wiki.Client` method. It carries its own `recoverPanic` and sits outside every `AllTools`-driven test; the scope note in `tools/annotation_coherence_test.go:14` says so and instructs that its annotations be kept coherent by hand. This exception is exhaustive. A second inline registration is a violation, not a new exception.

Why: 43 tools with hand-written registrations would drift into 43 slightly different error, logging, and audit behaviours, and the drift has already cost a tool: the `[1.28.1]` entry in `CHANGELOG.md` records `mediawiki_audit` silently failing to register for a missing case in the handler dispatch.

Codifies: `tools/definitions.go` (`AllTools`, `concatToolSpecs`), `tools/definitions_read.go`, `tools/definitions_write.go`, `tools/definitions_history.go`, `tools/definitions_quality.go`, `tools/registry.go` (`ToolSpec`), `tools/handlers.go` (`methodRegistrars`, `registerByName`, `register`), `rules/mcp-server-patterns.md` "Standard Project Structure".

**Enforcement: mechanically checked, with one hole.** `TestAllToolsRegistryIntegrity` (`tools/registry_test.go:82`) checks non-empty `Name`/`Method`/`Description` and unique names; `TestAllToolsNotEmpty` and `TestToolSpecMethods` (`tools/handlers_test.go:202, 221`) check required fields and that every spec's `Method` appears in a known-methods list. The hole: `TestToolSpecMethods` pins specs against a hand-maintained copy of the method list, not against `methodRegistrars` itself, so a method present in both the spec and the test map but missing from `methodRegistrars` does not fail any test — `registerByName` logs `"Unknown method, tool not registered"` at error level and the server starts without that tool. That is exactly the 1.28.1 failure shape, and it is still not test-covered. See Article IV, which this contradicts.

---

## Article II: Handlers never panic out; startup fails loudly

Every tool handler runs behind `defer h.recoverPanic(spec.Name, &err)`, and the enclosing closure MUST use **named return values** (`res *mcp.CallToolResult, out Result, err error` in `register`). Without named returns the deferred reassignment is a no-op and a recovered panic reaches the caller as `(nil, zero, nil)`, which an agent reads as a successful empty response — silently destructive for `EditPage`, `BulkReplace`, and `MovePage`. The panic value and stack are logged server-side; only a correlation ID reaches the caller. The doc comments on `register` and `recoverPanic` in `tools/handlers.go` state this requirement and cite HG-1 in `rules/code-review-prompts.md`.

This article is incident-born: the 2026-05-17 portfolio sweep found this repo's `recoverPanic` recovering and logging without writing to `*errPtr`, so a panic produced fake success. Fixed in PR #57 (commit `ecff360`) with the `*error` parameter and named returns; the pattern is tracked in `rules/review-patterns.md` "Silent-nil dispatcher".

Startup errors do not use this machinery: configuration and server failures exit via `log.Fatalf` (`main.go:272`, `main.go:392`, `main.go:421`, `main_httpserver.go:60`), which is loud by construction.

Codifies: `tools/handlers.go` (`register`, `recoverPanic`, `newCorrelationID`), `main_httpserver.go` (`recoverPanic` for the inline converter tool).

**Enforcement: partially mechanical.** `TestRecoverPanic` in `tools/handlers_test.go:128` exercises the registry recovery path, and `TestRecoverPanic` in `main_test.go:115` exercises the converter-tool path. Nothing checks that a future handler wrapper uses named returns; that rests on `register` being the only registration path (Article I) and on review.

---

## Article III: Anything that does I/O takes `context.Context` first

Every `wiki.Client` method that performs a network call MUST accept `context.Context` as its first parameter and propagate it through `apiRequest`, which also uses it to block on the rate-limit semaphore (`acquireRateLimitSlot`, `wiki/client_request.go`).

Of the 53 exported `Client` methods, the seven exempt are the in-process accessors that touch no network: `Close`, `CircuitBreakerStatus`, `DedupStats`, `SetAuditLogger` (`wiki/client.go:140-209`), `InvalidateCachePrefix` (`wiki/client_cache.go:237`), `SessionSnapshot` and `RestoreSession` (`wiki/session.go:36, 81`). This exemption list is exhaustive.

Two standing deviations, named so they are visible rather than implied: `isPrivateHost` resolves DNS via `net.LookupIP` with no context (`wiki/security.go:339`), and the PDF text extraction shells out via `exec.Command` rather than `exec.CommandContext` (`wiki/pdf.go:152`). Both are bounded operations on the request path, and both are the first places to fix if cancellation latency ever matters. They are not precedent for new context-free I/O.

Codifies: the 46 context-first methods across `wiki/*.go`; `wiki/client_request.go` (`acquireRateLimitSlot`, `apiRequest`).

**Enforcement: none mechanical.** No linter in `.golangci.yml` checks parameter ordering. The article rests on review and on `register[Args, Result]` requiring a `func(context.Context, Args) (Result, error)` signature, which forces the shape on every method a tool exposes.

---

## Article IV: Errors are never silently discarded

An operation MUST NOT swallow an error. If an error cannot be handled where it occurs it is logged with enough context to identify the failing object, and propagated or surfaced. A best-effort path that drops an error MUST document, at that line, why dropping it is safe.

This article is incident-born twice in this repo's own changelog. The `[1.28.1]` entry: `mediawiki_audit` silently failed to register — the server ran, the tool was simply absent, and nothing failed (the standing enforcement hole in Article I). The `[1.32.0]` entry: the MediaWiki edit API returns `result: "Failure"` inside a `200 OK` body when a CAPTCHA or AbuseFilter blocks an edit, and the CLI printed `Edited <page> (rev: 0)` — a false success on a write, discovered by an external user (@strk).

Codifies: `CHANGELOG.md` `[1.28.1]` and `[1.32.0]`; `rules/agent-interface-design.md` principle 6.

**Enforcement: mechanically checked.** `errcheck` is in the enabled linter list in `.golangci.yml` (line 13) and runs on every pull request via both `ci.yml` and `lint.yml`. Verified locally on 27-08-2026: `golangci-lint run ./...` reports **0 issues**. The `gosec` exclusion for `G104` carries the comment "Covered by errcheck" — and unlike the sister repo where that same comment sat over a disabled check, here it is true.

The honest bound: the config inherits the `std-error-handling` exclusion preset, which ignores unchecked returns from `fmt.Fprint*`-family writers. A no-preset probe (`golangci-lint run --no-config --default=none --enable=errcheck ./...`, 27-08-2026) reports exactly 5 hits, all human-terminal table output in the CLI: `cmd/wiki/comparetopic.go:76, 78`, `cmd/wiki/edit_captcha.go:50`, `cmd/wiki/translations.go:89, 92`. All five write to a tabwriter or terminal prompt where a failed write has no recovery; accepting them via the preset is deliberate. Zero hits in `wiki/`, `tools/`, or the server `main*.go` files.

---

## Article V: Every response is bounded, projected, and honest about truncation

No tool returns a raw upstream payload. API responses are projected into typed result structs before they leave the server. Every list operation is clamped through `normalizeLimit` (`wiki/client_helpers.go:33`). Content cut short carries a `Truncated` flag.

The current values are the contract: `DefaultLimit = 50`, `MaxLimit = 500`, `CharacterLimit = 250000` (`wiki/types.go:5-7`); per-tool tighter windows where the operation is expensive (for example `FindReplace` preview at default 10 / max 50, `wiki/findreplace.go:291`; related pages at 20 / 50, `wiki/related.go:154`); and `maxResponseBytes = 16 MiB` bounding any single upstream read so a hostile wiki cannot OOM the server (`wiki/client_request.go:25`). Page reads and batch reads set `Truncated` when `CharacterLimit` bites (`wiki/read.go:131`, `wiki/batch.go:72`). Raising a default or a cap is a change to this article's terms and belongs in an amendment, not a feature pull request.

Codifies: `wiki/types.go` (constants), `wiki/client_helpers.go` (`normalizeLimit`, `truncateContent`), `wiki/read.go` (`truncateIfNeeded`), `wiki/client_request.go` (`maxResponseBytes`); `rules/mcp-server-patterns.md` "Signal Density as a Cost Lens", where this server is a named reference for the character-limit idiom.

**Enforcement: partially mechanical.** `TestTruncateContent` (`wiki/validation_test.go:91`) covers the truncation helper. Nothing asserts that a *new* list tool routes its limit through `normalizeLimit`; that is convention plus review, and it is the article most likely to erode quietly.

---

## Article VI: Structured output, typed exit codes, loud failure on unknown input

Every MCP tool returns a typed result struct with named, stable fields, and list results carry explicit counts (`TotalHits` on search, `total_count`/`found_count`/`missing_count` on batch reads, `count` on listings — `wiki/types.go:59, 108-110`, `wiki/types_read.go:17, 76, 142`).

The `wiki` CLI returns typed exit codes so shell scripts can branch on failure category: 0 success, 1 default, 2 usage, 3 not found, 4 reserved for `wiki lint` findings, 5 API error, 6 auth, 7 rate limit, 10 config (`cmd/wiki/errors.go:14-23`). `wiki.APIError` values auto-classify by HTTP status in `ExitCode` (`cmd/wiki/errors.go:48`). Cobra flag-parse errors — including unknown flags — wrap to exit 2.

**Bounded scope.** Two things stated so the gap and the exception are visible:

1. **Explicit zero-result messages on the MCP surface.** The CLI prints `No results for %q` (`cmd/wiki/search.go:54`), but MCP tools return `total_hits: 0` with an empty array, and the `NewNoResultsError` constructor in `wiki/errors.go:315` is defined and never called from production code. `rules/agent-interface-design.md` principle 5 asks for a literal message; this server does not do it today, so this article does not claim it. Adding it (or deleting the dead constructor) needs code, not a sentence here.
2. **Missing configuration is a warning, not a refusal.** With `MEDIAWIKI_URL` unset the server starts in inspection mode — tools are listed, calls fail with a configuration error (`main.go:275`). This is deliberate, per the `[1.28.1]` changelog entry: MCP registries must be able to enumerate tools without credentials. It is the single permitted exception to fail-loud startup.

Codifies: `cmd/wiki/errors.go`, `main.go:268-275`, `wiki/types.go`, `wiki/types_read.go`; `rules/agent-interface-design.md` principles 4 and 6.

**Enforcement: mechanically checked for the CLI.** `TestExitCode` (`cmd/wiki/errors_test.go:11`), the collision guard asserting all eight codes stay distinct (`cmd/wiki/errors_test.go:56`), and the usage/config classification tests in `cmd/wiki/exitcodes_test.go`. Nothing asserts that a new MCP list result type carries a count field; that rests on review.

---

## Article VII: A tool description is a public contract with the agent

The description on a `ToolSpec` is the only thing an agent reads before deciding whether to call a tool. Changing it changes behaviour for every caller, invisibly, with no version bump and no error.

Descriptions follow the established shape and keep it: a first line stating what the tool does, then `USE WHEN`, `NOT FOR`, `PARAMETERS`, `RETURNS`, with `NOTE` and `WARNING` where applicable — the format is documented at the top of `tools/definitions.go`. Cross-references to sibling tools inside `NOT FOR` (for example `mediawiki_search` versus `mediawiki_search_in_page` versus `mediawiki_search_in_file`) are load-bearing disambiguation and MUST NOT be dropped when a description is shortened. Removing a `USE WHEN` clause, removing a `NOT FOR` cross-reference, or renaming a tool is a breaking change under Article XII, whatever happened to the code behind it.

Why: 43 tools include genuinely confusable groups, and the eval suite pins them: `evals/confusion_pairs.json` holds 10 confusion pairs, `evals/tool_selection.json` 37 selection tests, `evals/argument_correctness.json` 25 argument tests, run by `cmd/evals` against a live model.

Codifies: `tools/definitions*.go` (every spec), `evals/*.json`, `evals/runner.go`; `rules/mcp-server-patterns.md` "Context-Gap Tool Design".

**Enforcement: partially mechanical.** `TestAllToolsRegistryIntegrity` checks descriptions are non-empty. The eval suites parse and validate under `go test` (`TestLoadAllEvals`, `evals/runner_test.go:420`), but *running* them requires an API key and a live model, and no CI job does it. Nothing checks that a description edit preserved its `NOT FOR` cross-references. No test requires `WARNING` on destructive descriptions, though five of the write specs carry one today.

---

## Article VIII: Annotations tell the truth about what a tool does

`ReadOnly`, `Destructive`, `Idempotent`, and `OpenWorld` on a `ToolSpec` become MCP tool hints via `buildTool` (`tools/handlers.go`) that clients use to decide whether to prompt a human. A wrong hint costs a user their data.

A tool MUST NOT be both `ReadOnly` and `Idempotent`: idempotence carries meaning only for tools that change state, and asserting it on a read misleads a client reasoning about retry safety. Every tool that mutates wiki state is marked `Destructive: true` — including the two that do not look destructive at first glance, `mediawiki_manage_categories` (edits page content) and `mediawiki_upload_file` (writes attacker-controllable bytes; with `ignore_warnings` it overwrites existing files), whose specs carry HG-3 comments saying exactly why (`tools/definitions_write.go:265, 318`).

This article is incident-born: the `[1.30.0]` changelog entry (commit `c48289e`) records both of those tools shipping as `Destructive: false`, so hosts gating destructive operations on the annotation silently passed them through — on wikis allowing SVG, an overwrite becomes stored XSS as the wiki origin.

Codifies: `tools/registry.go` (annotation fields), `tools/handlers.go` (`buildTool`), `tools/definitions_write.go`, `CHANGELOG.md` `[1.30.0]`.

**Enforcement: partially mechanical.** `TestAnnotationCoherence` (`tools/annotation_coherence_test.go:17`) fails any spec setting both `ReadOnly` and `Idempotent`, over all of `AllTools`. No test forbids `ReadOnly` plus `Destructive`, and no test asserts that a state-mutating method is marked `Destructive` — the 1.30.0 mis-annotation would not be caught mechanically today. The inline `mediawiki_convert_markdown` sits outside the test's reach (Article I).

---

## Article IX: Every write carries a rationale and lands in the audit trail

The seven destructive tools (`mediawiki_edit_page`, `mediawiki_find_replace`, `mediawiki_apply_formatting`, `mediawiki_bulk_replace`, `mediawiki_upload_file`, `mediawiki_move_page`, `mediawiki_manage_categories`) require a `rationale` argument: a one-sentence agent-supplied "why", embedded via `BaseWriteArgs` where the field is schema-required, against `BaseArgs` where it is optional for the 35 declarative read tools (`wiki/types.go:21, 30`). The rationale is stored in the tool audit log (`tools/audit.go`, JSONL via `JSONToolAuditLogger`) and on the `mcp.tool.rationale` OTel span attribute, so post-hoc reconstruction of agent intent does not depend on the agent's own session surviving.

Shipping this was a deliberate breaking change (`CHANGELOG.md` `[1.31.0]`), and weakening it — making rationale optional on a write, or adding a write tool on `BaseArgs` — is a breaking change in reverse and an amendment to this article.

Codifies: `wiki/types.go` (`BaseArgs`, `BaseWriteArgs`, `GetRationale`), `tools/audit.go` (`extractRationale`, `newToolCallEntry`), `main.go:279-285` (`MEDIAWIKI_AUDIT_LOG` wiring).

**Enforcement: partially mechanical.** `TestExtractRationale` and `TestNewToolCallEntry_CapturesRationale` (`tools/audit_test.go:165, 350`) cover extraction and logging; the JSONL writer has validity and concurrency tests (`tools/audit_test.go:16-156`). The required-versus-optional split rests on which base struct a new Args type embeds; no test walks the destructive specs asserting their Args embed `BaseWriteArgs`. That is a review responsibility.

---

## Article X: Server-side fetches and access-granting operations fail closed

Four guards, each of which never falls open:

1. **The API client refuses all redirects.** The login flow POSTs the bot password; a 307/308 to another origin would re-POST it there. The configured wiki URL is the only legitimate target (`CHANGELOG.md` `[1.30.0]`, commit `1f8b2e4`).
2. **URL uploads pass a fail-closed domain allowlist.** `MEDIAWIKI_UPLOAD_ALLOWED_DOMAINS` unset or empty means every URL upload is rejected with an error naming the variable (`wiki/security.go:20-68`). Wildcards cover subdomains only, never the apex.
3. **URL fetches pass SSRF validation.** `validateFileURL` blocks private, loopback, link-local, and metadata addresses, failing closed on DNS errors; the download client re-validates every redirect hop (`wiki/security.go:227-351`).
4. **The HTTP server refuses to start bound to a non-loopback interface without an auth token** (`enforceBindSecurity`, `main_security.go:65`), and every aux endpoint except `/health` requires the bearer token when one is set.

Why: wiki content and upload URLs are attacker-influenceable, and an MCP server is a confused-deputy target. The allowlist and SSRF guards are the server-side rails the agent cannot be talked out of.

Codifies: `wiki/security.go`, `wiki/write_files.go`, `wiki/client.go:121` (the API client's `CheckRedirect` returning `http.ErrUseLastResponse`), `main_security.go`, `SECURITY.md` "SSRF Protection", `rules/mcp-server-patterns.md` "Visibility Before Enforcement"; graduated gates HG-3 and HG-4 in `rules/code-review-prompts.md`.

**Enforcement: mechanically checked — the best-enforced article in this document.** `wiki/upload_allowlist_test.go` (fail-closed on unset and whitespace env, exact host, wildcard, malformed URL — 7 tests), `wiki/client_redirect_test.go` (`TestAPIClientRefusesCrossOriginRedirect_307` and `_308`), `wiki/security_test.go`, `TestEnforceBindSecurity` and `TestAuxEndpointsRequireAuth` (`main_test.go:218, 248`). All run in CI via `go test -race ./...`.

---

## Article XI: No credentials in version control, and none echoed to callers

API bot passwords and tokens MUST NOT be committed anywhere in this repository: not in code, not in documentation examples, not in fixtures. Configuration comes exclusively from `MEDIAWIKI_*` environment variables (`wiki/config.go`); every documentation example uses a placeholder.

The same discipline applies at runtime: debug logging redacts `logintoken`/`csrftoken` values before a response body is previewed (`tokenPattern`, `wiki/client_request.go:19-20`), and API errors reaching MCP callers are structured `APIError` values that carry HTTP status and stable text while the body snippet — which can echo POSTed parameters — is capped at 256 bytes and kept server-side only (`wiki/errors_api.go`, `CHANGELOG.md` `[1.30.0]`, commit `bef66aa`).

Codifies: `.gitignore` (`.env`, `.env.local`, `notes/`), `wiki/config.go`, `wiki/client_request.go`, `wiki/errors_api.go`, `SECURITY.md` "Secure Credential Storage".

**Enforcement: partial, and honest about the gaps.** `TestAPIError_BodySnippetTruncation` (`wiki/client_apierror_test.go:143`) pins the 256-byte cap. There is no secret scanner in any workflow, `gosec`'s `G101` is excluded in `.golangci.yml` with a documented false-positive reason, and the `tokenPattern` redaction has no test. The committed-credential half of this article rests on `.gitignore` and review.

---

## Article XII: Semantic versioning, and the changelog gates the release

The released binaries (`mediawiki-mcp-server` and the `wiki` CLI), the tool set, and every tool's arguments are versioned artifacts published to GitHub releases, GHCR, and the MCP Registry — where a published version is permanent and duplicates are rejected (`.github/workflows/mcp-registry.yml` header comment). Breaking changes MUST NOT ship in a patch or minor release.

On this server, "breaking" means any of: removing or renaming a tool, making an optional argument required (the `[1.31.0]` rationale change is the precedent, correctly labelled **Breaking**), narrowing accepted values, removing a result field, changing a documented CLI exit code, or the description changes named in Article VII.

Every user-visible change is recorded in `CHANGELOG.md` under the version that ships it, and the reported version is injected at build time from the git tag (`-X main.ServerVersion=...`, `release.yml:66`, `Makefile:8`) — hand-maintained version constants are banned since `[1.32.1]`, when `ServerVersion` sat stuck at 1.28.1 across four releases.

Codifies: `CHANGELOG.md` (Keep a Changelog format, incident-grade entries), `.github/workflows/release.yml`, `.github/workflows/mcp-registry.yml`, `server.json` (version patched by the registry workflow at publish time, so the committed value is not load-bearing).

**Enforcement: mechanically checked at the release boundary, as of 26-08-2026.** The `changelog` job in `release.yml:13-27` fails any tag whose version has no `## [<version>]` section in `CHANGELOG.md`, and the error message names its own origin: v1.33.0 shipped unstamped and was back-filled. Builds depend on that job, so an unstamped release cannot ship. No CI job checks that a *pull request* touching `tools/definitions*.go` also touched `CHANGELOG.md`; within a release cycle the discipline rests on practice.

---

## Article XIII: The supply chain is verified on every pull request

CI MUST verify, on every push to `main` and every pull request against it: that neither `go.mod` nor `go.sum` drifts from `go mod tidy`, that tests pass with the race detector, that `golangci-lint` reports no issues, and that `gosec` and `govulncheck` run.

The tidy check diffs **both** files (`git diff --exit-code go.mod go.sum`, `ci.yml` `go-mod-tidy` job). This is deliberate and not redundant: a `go get` before the import lands records the module as `// indirect` in `go.mod`, build and test pass either way, and a check diffing `go.sum` alone reports clean. Two sister repositories were caught by exactly this on 30-07-2026 (`rules/mcp-server-patterns.md` "Supply Chain Security").

Branch protection names the `all-checks` aggregator job, never a matrix leg: a leg's context embeds its matrix values, so changing the Go version renames the context and branch protection waits forever for a name nothing emits — the failure that blocked every PR merge on the sister miro repo from 23-12-2025 to 26-08-2026, per the comment in `ci.yml`. The aggregator runs under `if: always()` and fails on any dependency result that is not exactly `success`, because GitHub counts a skipped required check as satisfied.

Codifies: `.github/workflows/ci.yml` (lint, test with `-race`, gosec, govulncheck, go-mod-tidy, five-platform build matrix, `all-checks`), `.github/workflows/lint.yml` (golangci-lint v2.12.2).

**Enforcement: mechanically checked, with two steps that cannot fail the build.** `govulncheck` runs with `|| echo "::warning::..."`, so a known vulnerability produces a warning annotation and a green build — the stated rationale is that stdlib findings resolve with Go patch updates, and the cost is that a vulnerable direct dependency is equally unable to redden CI. Coverage upload sets `fail_ci_if_error: false`, which is correct for a reporting step. One gap: `go mod verify` (checksum verification) is not run anywhere in CI.

---

## Articles considered and rejected

**Test-first development, with tests required on every exported function.** Rejected because it is not what this repository does and stating it would make the document a wish. Coverage is real but uneven: `wiki/` carries 34 test files including redirect, allowlist, and API-error suites, while `converter/` and the `cmd/wiki` command surface are thinner. CONTRIBUTING.md asks for tests with each new tool, which is the honest form, already written where it belongs.

**No mocks in integration tests.** Rejected because the article would describe a check that cannot gate anything. This repository actually has a real integration suite — `wiki/integration_test.go` behind a `//go:build integration` tag, run by `.github/workflows/integration.yml` against a live MediaWiki container — but the workflow is `workflow_dispatch` (manual) only, so no pull request is ever gated by it. An article about a suite that runs only when someone remembers to run it would claim enforcement that does not exist. Worth revisiting if the integration workflow ever gets a schedule.

**Fixtures are captured from live responses, never imagined.** Rejected as not applicable in the shape the sister miro repo needed it. That repo wraps an evolving proprietary API and carries an `api-tracking/` probe pipeline; this repo wraps the MediaWiki Action API, which is stable, publicly documented, and covered here by unit tests against `httptest` mocks plus the on-demand integration suite above. There is no fixture-provenance incident in this repo's changelog to anchor the article, and inventing one would violate this document's own ground rule.

**Structured logging with `log/slog` everywhere.** Rejected as written because the repository is mixed on purpose: `slog` throughout the server and `wiki/` request path, `fmt.Printf`/tabwriter output in `cmd/wiki` where printing tables to a human at a terminal is the point, and `log.Fatalf` at the five fatal-exit sites in `main.go` and `main_httpserver.go`. An article would need to carve out the whole CLI, at which point it constrains almost nothing. The errcheck probe in Article IV names the exact five CLI print sites this exemption covers.

**Explicit zero-result messages on every list.** Rejected as a standalone article because the MCP surface does not do it — and the repo contains the evidence of the intent without the follow-through: `NewNoResultsError` (`wiki/errors.go:315`) is defined and never called from production code. Recorded instead as a named, bounded gap inside Article VI, which makes it visible without pretending it is policy.

**A rule requiring the README/TOOLS.md tool count to match the registered count.** Rejected as too small for a constitution. The drift risk is real — both files hand-maintain "43 tools", the declarative count is 42 plus one inline registration, and no test pins the total (`TestAllToolsNotEmpty` accepts any non-zero count) — but the fix is a `TestToolCount` pinned at 43, not an article. Worth writing that test.

**Contribution-doc accuracy as a governed surface.** Considered because CONTRIBUTING.md "Adding New Tools" still describes a `registerToolHandler` call in `main.go` that no longer exists (the real path is Article I's spec-plus-registrar), and `wiki/errors.go` has since split into `errors_api.go` and siblings. Rejected: stale internal docs mislead contributors but break no external consumer, which fails this document's one-line test. It belongs in an ordinary docs fix, not an article.

**Secure defaults as a general principle.** Rejected as too broad to check. Its concrete instances here are strong enough to stand as their own articles with their own test files: the four fail-closed guards of Article X and the bind refusal. A general "prefer the restrictive option" adds words without adding a decidable rule.

---

## Amendment log

| Date | Change |
|------|--------|
| 27-08-2026 | Ratified. Thirteen articles, adapted from the `CONSTITUTION.md` in `gridctl/gridctl` (Apache-2.0, github.com/gridctl/gridctl) via the portfolio template. |
