# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Proksi HTTP is a single-binary reverse proxy for shadow testing (in development, NOT production-ready). Each request is forwarded synchronously to a `main` upstream; subject to a probability gate it is also mirrored to a `test` upstream, the two responses are compared, and mismatches are persisted to a storage backend (stdout or Elasticsearch).

Proksi is a toolkit of per-protocol proxies. Only `http/` exists on `main`. The `dev/redis` branch adds `redis/` (+ `internal/rediscluster`, with tests) and `dev/sql` adds a dummy `sql/`; each has its own `make build-*` target, `<component>/config.example.yaml` and `internal/config/<component>.go`. Those two are temporary feature branches (not integration branches) that get merged into `main` by PR once the component is usable; until then, branch from them for Redis/SQL work.

## Commands

```sh
make build-http          # CGO_ENABLED=0 go build -o proksi-http ./http
make test                # go test -race ./... (unit + e2e tests with httptest upstreams and a fake storage)
make lint                # golangci-lint run ./... (.golangci.yml, v2 format)
make check-suite         # test + lint; what CI runs
./proksi-http -config http/config.example.yaml
docker compose -f docker-compose.dev.yaml up   # local Elasticsearch :9200 + Kibana :5601 only
```

Run a single test (once tests exist): `go test ./http -run TestName`.

`proksi-http` and `proksi-redis` in the repo root are gitignored build artifacts, not source.

## Architecture

- `http/http.go` is `package main` and holds almost all logic: the server, `server.handle`, the worker pool, `upstreamTestJob.Do`, and the comparison functions (`bodyEqualizerFunc`, `JSONBytesEqual`, ...). Everything reusable lives under `internal/` (`config`, `logging`, `metrics`, `storage`).
- Wiring is via package-level globals (`config.HTTP`, `logging.L`, `strg`), not dependency injection. Code reads `config.HTTP.*` directly; per-route settings are read through `config.GetRouteConfig(route)` / `config.IsRouteSkipped(route)`, backed by the pre-computed `config.ComputedConfigs` (built by `HTTPConfig.Finalize()`, which `LoadHTTP` calls; tests that hand-build a config start from `config.DefaultHTTP()` and call `Finalize()`).
- Mirroring is asynchronous: `handle` pushes an `upstreamTestJob` onto a buffered `jobs` channel drained by `Worker.Count` goroutines. The test request uses a fresh `context.Background()`, not the client's, and request/response bodies are buffered so readers can be seeked back to the start for replay and comparison.
- Comparison order: status code, then response headers (if the route's `compare_headers`; all headers except `skip_headers` and `Content-Length`, which is always ignored), then body (if the route's `compare_body`). The body comparator is chosen by the main upstream's content-type (JSON uses `reflect.DeepEqual`, XML types use the semantic comparator in `http/xml.go`, everything else uses `bytes.Equal`). On JSON mismatch, the route's `skip_json_paths` are overwritten with `sjson` on both sides and compared again; an `sjson` error is logged and the original bodies are kept. Each mismatch is stored with a `comparison_type` (`status_diff`, `header_diff`, `body_diff`).
- Per-route config: the request key is `METHOD:/path` (`config.FormatRoute`). `skip_routes` entries are served by the main upstream but never mirrored. `route_configs` entries override `global_config` per route; options are `"enable"`/`"disable"`/`""` (inherit) strings, `skip_headers` and `skip_json_paths` append to the global lists, and `test_probability` `0` means inherit (#80). Patterns: `*` as a method matches any method, a single `*` segment matches one path segment, and a trailing `/*` matches that prefix and everything below it on a segment boundary. When several patterns match, the longest wins, then a concrete method over `*`, then lexicographic order. The legacy top-level `skip_json_paths`, `test_probability`, `log_response_payload` and `compare_headers` still work and are migrated into `global_config` only when `global_config` is left at its defaults. Full reference: `doc/route_configuration.md`.
- Storage backends implement `Store(Log) error` (`internal/storage`); the backend is chosen by `storage_type`.
- Config is loaded with koanf: defaults struct, then the YAML file, then unmarshal. Keys are snake_case. `LoadHTTP` returns `(*HTTPConfig, error)` and `main` fatals on the error.

Config keys (YAML, snake_case): `bind`, `metrics.{enabled,bind}`, `storage_type` (`stdout`|`elasticsearch`), `upstreams.{main,test}.{address,timeout,dial_timeout,tls_handshake_timeout,response_header_timeout}` (duration strings, `0` disables), `worker.{count,queue_size}`, `elasticsearch.*`, `log_level` (`debug`|`info`|`warn`|`error`|`fatal`, default `info`), `global_config.{compare_headers,compare_body,skip_headers,store_req_body,store_resp_bodies,skip_json_paths,test_probability}` (defaults `true`, `true`, `[Date, Server]`, `false`, `true`, `[]`, `100`), `skip_routes`, `route_configs.<METHOD:/path>.*` (`test_probability` there is a `*uint64`: absent = inherit, `0` = never mirror the route), and the legacy `skip_json_paths`, `test_probability` (0-100), `log_response_payload`, `compare_headers`. `LoadHTTP` calls `HTTPConfig.Validate()` and returns an error on invalid values (upstream and Elasticsearch addresses must be http(s) URLs with a host, timeouts non-negative, `log_level` known). Defaults live in `internal/config/config.go`; `http/config.example.yaml` is the documented sample.

## Git workflow

Trunk-based: `main` is the only long-lived branch.

- `main` is always releasable and green. Feature branches are short-lived, cut from `origin/main`, PR'd back into `main` and **squash-merged**. No merge commits, no back-merges.
- Releases are tags on `main` (`vX.Y.Z` for HTTP); a tag push runs the `test` workflow, then builds and publishes the Docker image and Helm chart. `main` may contain unreleased changes: the latest tag, not `main`, is what is released.
- Hotfix: if `main` has moved on and an old release needs a fix, branch from the release tag (`hotfix/vX.Y.Z`), PR it, tag from that branch, then port the fix to `main`. Delete the hotfix branch afterwards.
- Unfinished components (`dev/redis`, `dev/sql`) are ordinary feature branches. Keep them up to date with `main` and PR them into `main` when ready; do not add new `dev/*` branches.
- **Nothing is pushed directly to `main`; everything lands through a pull request** (enforced by a GitHub ruleset). That includes docs, `.gitignore`, CI and repo config.
- The `test` CI check (`go test` + golangci-lint, config in `.golangci.yml`, version pinned in `.github/workflows/test.yaml`) is required on `main`. The `main` ruleset blocks deletion and force-push. "Delete branch on merge" is enabled, so merged feature branches are removed automatically. Never disable the deletion rule on `main`.
- Tag scheme: HTTP releases use `vX.Y.Z` (existing tags). When another component is first released, use `<component>/vX.Y.Z` for it and scope the release workflow by tag prefix.

## Rules for Claude

- **Always work in a git worktree**, never by switching branches in the primary checkout, so several agents can work on the repo at once. Create it as a sibling of the repo (not inside it):
  `git fetch origin && git worktree add ../proksi-worktrees/<branch> -b <branch> origin/<base>`
  Remove it with `git worktree remove` once the branch is merged.
- **Base branch:** everything branches from `origin/main`, except work on a component that has not landed on `main` yet (Redis: `origin/dev/redis`, SQL: `origin/dev/sql`), which branches from and PRs into that branch.
- **Always `git fetch origin` immediately before creating a branch and base it on the remote ref (`origin/<base>`)**, never on a local branch that may be stale.
- Open feature PRs against the branch they were cut from (normally `main`) and squash-merge them.

- **Track every issue you find as a GitHub issue, not in documentation.** Bugs, gaps, missing tests, tech debt and follow-ups noticed while working (even in code you are not touching) go in a ticket created with `gh issue create` (label `bug` or `enhancement`; search `gh issue list` first to avoid duplicates). Do not record known bugs or TODOs in CLAUDE.md, the README or code comments, and do not leave `t.Skip` reasons without an issue number. Reference the ticket from the PR. Use `Closes #N` in the PR description so merging into `main` closes the issue; for PRs into a non-default branch (e.g. `dev/redis`) GitHub will not auto-close, so close them manually with a comment linking the PR.

## Gotchas

- `test_probability` is `uint64`; the gate is `counter%100 < TestProbability` (0 = off, 100 = all), validated to 0-100 at load.
- Each request carries an `X-Request-Id` (propagated or generated), set before the header clone so both upstreams see it, and stored as `request_id` in `storage.Log`. Hop-by-hop headers are stripped. Shutdown on SIGINT/SIGTERM drains the job queue (30s timeout).
- Upstream clients are built by `newUpstreamClient` (`http/client.go`) and are package globals; tests that use the harness must set them (see `http/helpers_test.go`). `flag.Parse()` runs in `main()`, not `init()`, so `go test` works.
- Metrics: `proksi_http_request_count`, `proksi_http_request_duration`, `proksi_http_mismatch_count{type}`, `proksi_http_queue_depth`, `proksi_http_comparison_results`, `proksi_http_route_skips`, `proksi_http_status_2xx_vs_non2xx_count` (see README).
- `go.mod`, CI (`setup-go` reads `go.mod`) and the Dockerfile must stay on the same Go version (1.25.0); a stale `go.mod` made `redcon` fail to build in CI (#35). Tag releases are gated on the `test` workflow, then publish the Docker image and the Helm chart (`oci://ghcr.io/snapp-incubator/charts/proksi`, chart version = tag without the `v`, `appVersion` = the tag).
- The runtime image ships no config. It expects one mounted at `/usr/local/etc/config.yaml` (the Helm chart supplies it via ConfigMap). Local `config*.yaml` files are gitignored, except `http/config.example.yaml`.
- Helm values in `.deploy/proksi/` use camelCase (`testProbability`, `skipJsonPaths`), while the app config uses snake_case.
