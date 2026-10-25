# Pinned mihomo source with two concurrency fixes

**Modified by fly on 2026-10-25.** The copied core and these modifications retain the upstream GNU General Public License v3 (see `mihomo/LICENSE`).

`mihomo/` is a complete source copy of **github.com/metacubex/mihomo v1.19.33** from the Go module archive. The root `go.mod` uses a local `replace` so a normal Go build includes the reviewed fixes without changing a user's shared module cache, installing patch tools, or fetching an unpinned fork. Upstream license and notices are retained in that directory.

The source version and other dependencies are unchanged. Only these upstream files differ:

1. `log/log.go`: atomic reads/writes for the global log level. Reload calls `SetLevel` while provider health checks and finalizers log concurrently.
2. `listener/mixed/mixed.go`: an atomic closed flag between `Close` and the accept loop. This also covers a TCP listener closed after a UDP bind conflict.

The small source delta is recorded in [patches/mihomo-concurrency.patch](patches/mihomo-concurrency.patch). Keep this patch and provenance current when upgrading the core; do not make unrelated edits to the copied source.

Evidence: GitHub race checks exposed the log race; `go test -race -count=10 ./internal/core` reproduced both races. `TestLogLevelConcurrentWithHealthMessages` additionally reproduces the log race with fixed local inputs. After the patch, rerun this controlled test, repeated core tests and the full project race suite.

This changes dependency source delivery only. Normal users continue using `make install` and the same fly command, proxy port and GUI.
