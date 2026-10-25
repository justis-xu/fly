# fly Portability and Availability Implementation Plan

> **For agentic workers:** Execute inline via superpowers:executing-plans. User explicitly authorized direct fixes and final commit/push; no repeated design approval. Final independent reviewer required.

**Goal:** 让新用户在独立电脑环境下成功配置订阅、规则、节点，并分清延迟与网站可达性。
**Architecture:** 保留 Go 嵌入 Next 静态页面；下载与缓存、用户规则、异步检查各自小模块，通过现有同源 API 使用。
**Tech Stack:** Go / mihomo v1.19.33（现有并发补丁）、Next / React / shadcn/ui。
**Spec:** docs/superpowers/specs/2026-10-25-portability-design.md

## Global Constraints

- 单管理页，默认代理 7888 / 管理 7889，无系统代理或 TUN，无新增常驻进程。
- 不依赖当前用户的 HOME、Clash、环境代理、订阅或缓存；不把凭据写入截图、构建产物或测试记录。
- 所有输入先校验；失败保留可用配置；用户自定义规则与节点选择按约定持久化。
- 只做与上述验收相关的改动；不可把窄范围测试当成广泛可用性证明。

### Task 1: subscription loading and independent environments

**Files:** internal/sub/{fetch,parse,cache}*, internal/config/*, internal/app/*, main.go; internal/storage/* if needed.
**Interfaces:** FetchWithOptions(ctx,url,options) returns bounded body / metadata / download route; URL-bound cache; App.Load/Refresh/ChangeSubscription/ImportSubscription return explicit errors and commit accepted state only.

- [x] RED: TestFetchIgnoresEnvironmentProxy, TestFetchFallbackProxy, TestOversizedSubscriptionRejected, TestCacheDoesNotCrossSources; `go test ./internal/sub` must fail against current behavior.
- [x] GREEN: explicit transports, bounded retry/context, 8 MiB + 1 detection, classified redacted errors, atomic source-bound cache.
- [x] RED/GREEN: reserved/duplicate/unnamed nodes and unsupported critical sharing parameters; use actual mihomo adapter validation.
- [x] RED/GREEN: failed new URL preserves persisted old URL/runtime; partial node filtering; imported file survives offline restart; FLY_HOME isolate path.
- [x] Add provider source expansion with the same bounded download policy; disallow recursive or unsupported file references with useful errors.
- [x] Run package tests and full race/vet; ledger exact results.

Representative invariant:
```go
if err := a.ChangeSubscription(invalidURL, ""); err == nil { t.Fatal("invalid replacement accepted") }
if a.SubscriptionURL() != oldURL { t.Fatal("failure lost working subscription") }
```

### Task 2: inspectable and editable routing

**Files:** internal/config/user_rules.go, internal/app/routing.go, internal/core/routing.go, internal/server/routing.go and tests.
**Interfaces:** RuleOverride{match,value,action}; GET/PUT /api/routing; GET /api/rule-providers/{name}?q=&offset=&limit=; validated overrides prepend to effective config and persist separately.

- [x] RED/GREEN: domain/URL/wildcard normalization, CIDR validation, invalid action / duplicates, explicit first-match order.
- [x] RED/GREEN: invalid save retains previous overrides and live config; successful edits survive subscription refresh and restart.
- [x] Implement searchable effective rules and provider payload reading including domain/ipcidr MRS strategies where available; paginate, bound response, never expose credential URLs.
- [x] Exercise direct/proxy/reject routing against controlled HTTP targets through real mixed listener.

### Task 3: asynchronous latency and website checks

**Files:** internal/checks/{manager,probe}.go and tests; internal/core/probe.go; internal/server/checks.go.
**Interfaces:** POST/GET/DELETE /api/checks; mode latency/sites; pinned proxy and per-result timestamps/status/code/delay/message; one bounded job at a time, cancellation and partial results.

- [x] RED/GREEN: completed/pending/canceled distinct, worker limit, context timeout, stale results invalidated on subscription change, concurrent start rejected.
- [x] RED/GREEN: direct node probe traverses actual adapter; local mixed probe exercises rules; local target responds 200/204/403/429/500, closed socket, redirect and timeout.
- [x] Keep legacy /api/test compatibility or document its new response; no test result may silently select a different node or mark untested nodes failed.
- [x] Test website and latency independence, selected automatic strategy pinning, and meaningful errors without secrets.

### Task 4: usable single-page controls

**Files:** web/src/components/{nodes,subscription,routing,checks,dashboard}*, web/src/lib/*, web/tests/*; web/scripts/preview.mjs.
**Interfaces:** consume Task 1-3 APIs; persisted sorting preference; dialogs for rule editing and website tests, main screen stays focused.

- [x] RED/GREEN React tests: sorting with missing/failed delay, search, import and download proxy, failed transactional save, editing/reordering rules, job progress/cancel, restricted vs failed site results.
- [x] Replace generic green service claim with clear local service / selected node / last check context and recovery actions.
- [x] Capture safe desktop and 390px screenshots of final paths, verify keyboard and scrolling, console/network clean.
- [x] Build/export static UI and check source/output parity.

### Task 5: clean-room acceptance, manuals and delivery

**Files:** scripts/acceptance/*, docs/{INSTALL,USAGE,PROXY_SWITCHER,FAQ,QA,DEVELOPMENT}.md, docs/images/*, CI workflow.
**Interfaces:** process-level acceptance takes built binary, creates its own data/ports/fixture servers, cleans only owned resources.

- [ ] New HOME + no proxy env + no previous cache; run/install/stop; add source A then B; invalid replacement, offline restart and cache boundaries; override persistence and HTTP/SOCKS behavior.
- [ ] Update illustrated novice steps for supported subscription formats, blocked subscription download, importing files, healthy-node recovery, editable rules, sorting and two test types.
- [ ] Recheck commands and image/anchor links; redact private configuration and examine real images.
- [ ] Full frontend tests/build/types; Go race/vet and process acceptance; independent whole-change review and fixes; final install/restart as authorized.
- [ ] Stage all, commit, push, GitHub CI passed, remote/local equality and clean tree.
- [ ] Completion audit: every numbered spec item has concrete current evidence; only then mark goal complete.

## Execution ledger

- 2026-10-25 baseline: clean master 276dcc9. Current fetch uses inherited environment proxy; URL saved before validation; cache unbound; node list has search only; generic delay URLTest does not verify website status. Goal is active; none of the new requirements is yet accepted as complete.
- Interfaces preflight: subscription transaction owns core replacement; rule mutation and checks must serialize or pin snapshots across replacement; sorting consumes timestamped results, never an absent result as zero.

- Task 1 and Task 2 backend implementation: new download transport ignores environment/global proxy; bound atomic cache and transactional URL/file activation; FLY_HOME; partial node validation/provider expansion; normalized user rule editing and persistent precedence; provider YAML/text/MRS search/pagination; real mixed-port direct/proxy/reject proof. API exists; UI/docs remain Task 4/5.
- Concurrency evidence: real reloads exposed inbound prefix slices and IP4P flag; pinned selection exposed history queue empty-check and selector name races. Each controlled regression was observed RED under -race before its minimal upstream fix. Seven upstream files now differ from v1.19.33 (including two baseline files); provenance/patch updated.
- Current Go race suite: all packages pass; go vet passed. Process-level new-HOME acceptance and final review remain outstanding.
- Ruling: HTTP rule resources now refresh with fly subscription updates (manual/24h), using the same download fallbacks; core provider interval is 0. This avoids independent background downloads bypassing the configured fallback, makes resource failure visible, and keeps immutable strategies between reloads. Cost if wrong: a subscription's shorter per-rule-provider interval is superseded by fly's refresh schedule; documented in Task 5.

- Task 3 backend: async /api/checks, bounded workers/deadline/cancel, timestamps, generation invalidation, fixed-node website GET headers and actual mixed-rule path, strict manual HEAD/204 latency. Legacy /api/test reports only completed items. Current selected automatic health is separate from local listener readiness. Website probes never mutate auto/latency histories. Real local relay tests verify pinning and mixed direct/proxy behavior; HTTP 403/429/500, redirects, timeouts verified with local targets. Full Go race/vet passed; UI integration is Task 4.

- Task 4: UI has persisted four-way sorting, independent diagnostic jobs/progress/cancel, stale-generation guards, transactional URL/file + masked backup proxy inputs, editable/reorderable normalized user rules, rule-set search, and separate local-ready/current-node status. React 21 tests passed; build and types passed. Browser fixture QA exercised name/latency sorting, persisted option after reload, rule save, provider search, file chooser/read/import, switching source, latency cancel, and website response classes. 390px viewport/main and diagnostic dialog had document width exactly 390; viewport reset. Console warnings/errors empty. New screenshots are genuine JPEG files with correct extensions; extension native PNGs unchanged. Final process acceptance/manual rewrite still Task 5.
- Ruling: automatic node health checks run every 300 seconds even with a manually selected node, so manual-node failures can become visible. It never switches a manual selection. Cost if wrong: background checks create periodic HEAD traffic; document the target, timing and distinction from website tests.

- Task 5 preparation complete: real clean-room install/runtime script passed with fresh HOME/FLY_HOME, standard system PATH, no proxy environment; local authenticated resources, HTTP/SOCKS forwarding, invalid URL/core-rule/CLI rollback, explicit backup, source B offline restart, persistent rules, import/restart, private modes and owned loopback listeners. Added to CI. Full Go race/vet/build and 22 React tests/build/types passed. Manuals now cover every new path; local document/image links checked. Independent final review, deployment and push/CI gates remain.
- Additional actual regressions fixed before review: malformed top-level rules were silently discarded; GLOBAL could bypass selected node; broad name filters deleted valid names; provider auth headers were lost; IP overrides skipped unresolved domain destinations; non-HTTP sharing URIs escaped error redaction; stale PID could identify an unrelated process. Each issue had an observed failing controlled check before its fix.
- Subscription activation now writes a separate atomic cache for each source before persisting its address, keeping old and new sources recoverable across process interruption; disk-write failure regression verifies original live selector, saved URL and cache remain.
- Ruling: preserve pre-upgrade legacy cache availability but carry an explicit unverified-source marker across restarts until a successful online fetch. Existing legacy data lacks provenance, which cannot be reconstructed. Cost if wrong: old cached nodes may belong to an older source; UI and manual warn and require successful update to verify. A new replacement URL never borrows that cache.
- Final review focus: transactions/cold cache/failed persistence; provider auth/fallback and unsupported source fields; rule normalization/priority/provider contents; cancellation/provenance and stale generation; lifecycle identity across environment changes; genuine novice documentation and clarity between readiness/latency/websites. Windows is explicitly outside the existing Unix/macOS delivery scope.
