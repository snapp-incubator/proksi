# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Proksi HTTP is a single-binary reverse proxy for shadow testing (in development, NOT production-ready). Each request is forwarded synchronously to a `main` upstream; subject to a probability gate it is also mirrored to a `test` upstream, the two responses are compared, and mismatches are persisted to a storage backend (stdout or Elasticsearch).

Proksi is a toolkit of per-protocol proxies. Only `http/` exists on `main`. `dev/redis` adds `redis/` (+ `internal/rediscluster`, with tests) and `dev/sql` adds a dummy `sql/`; each has its own `make build-*` target, `<component>/config.example.yaml` and `internal/config/<component>.go`. Check out the relevant `dev/*` branch before working on those.

## Commands

```sh
make build-http          # CGO_ENABLED=0 go build -o proksi-http ./http
make test                # go test ./...  (no *_test.go files exist yet)
make lint                # golangci-lint run ./... (default config)
make check-suite         # test + lint; what CI runs
./proksi-http -config http/config.example.yaml
docker compose -f docker-compose.dev.yaml up   # local Elasticsearch :9200 + Kibana :5601 only
```

Run a single test (once tests exist): `go test ./http -run TestName`.

`proksi-http` and `proksi-redis` in the repo root are gitignored build artifacts, not source.

## Architecture

- `http/http.go` is `package main` and holds almost all logic: the server, `server.handle`, the worker pool, `upstreamTestJob.Do`, and the comparison functions (`bodyEqualizerFunc`, `JSONBytesEqual`, ...). Everything reusable lives under `internal/` (`config`, `logging`, `metrics`, `storage`).
- Wiring is via package-level globals (`config.HTTP`, `logging.L`, `strg`), not dependency injection. Code reads `config.HTTP.*` directly.
- Mirroring is asynchronous: `handle` pushes an `upstreamTestJob` onto a buffered `jobs` channel drained by `Worker.Count` goroutines. The test request uses a fresh `context.Background()`, not the client's, and request/response bodies are buffered so readers can be seeked back to the start for replay and comparison.
- Comparison order: status code, then content-type (if `compare_headers`), then body. The body comparator is chosen by the main upstream's content-type (JSON uses `reflect.DeepEqual`, everything else uses `bytes.Equal`). On JSON mismatch, `skip_json_paths` are overwritten with `sjson` on both sides and compared again. The XML comparator is a stub that returns `false`.
- Storage backends implement `Store(Log) error` (`internal/storage`); the backend is chosen by `storage_type`.
- Config is loaded with koanf: defaults struct, then the YAML file, then unmarshal. Keys are snake_case. `LoadHTTP` fatals on error rather than returning one.

Config keys (YAML, snake_case): `bind`, `metrics.{enabled,bind}`, `storage_type` (`stdout`|`elasticsearch`), `upstreams.{main,test}.address`, `worker.{count,queue_size}`, `elasticsearch.*`, `skip_json_paths`, `test_probability` (0-100), `log_response_payload`, `compare_headers`. Defaults live in `internal/config/config.go`; `http/config.example.yaml` is the documented sample.

## Git workflow

- `main` holds released code; releases are tags (`vX.Y.Z`) on `main`, and a tag push builds and publishes the Docker image.
- Each product has a long-lived integration branch: `dev/http`, `dev/redis`, `dev/sql`. These are permanent; do not delete them.
- Feature branches are cut from the relevant `dev/<component>` and PR'd back into it, never straight into `main`. **Squash-merge** feature PRs into `dev/*`.
- To release, merge `dev/<component>` into `main` with a **merge commit** (no squash or rebase), so `main` keeps per-release history. Tag the release on `main`.
- Shared changes that are not logic (docs, `.gitignore`, CI, repo config) may be committed directly to `main`; then merge `main` into every `dev/*` branch. Logic changes always go through a `dev/*` branch.
- `dev/*` must always be at or ahead of `main`: after anything lands on `main` (release, docs or hotfix), merge `main` back into every `dev/*` branch.
- No redis release has been cut yet, so the tag scheme for non-HTTP components is undecided.

## Gotchas

- `test_probability` is `uint64` and the gate is `counter%100 < TestProbability-1`, so `0` underflows and enables mirroring (#27).
- The upstream `http.Client`s have no timeouts (#28); request headers are shared by reference (#29); multi-value headers are collapsed into one `"[v1,v2]"` value (#1); a `panic` in the `skip_json_paths` loop can crash a worker (#30).
- `internal/metrics/http.go` has dead code (a standalone `func main()` and `recordMetrics()`, #33); the real entry point is `InitializeHTTP`.
- Version skew: `go.mod` says Go 1.18 (and CI uses it), while the Dockerfile builds with `golang:1.25.0` (#35). The XML comparator is a stub (#32).
- The runtime image ships no config. It expects one mounted at `/usr/local/etc/config.yaml` (the Helm chart supplies it via ConfigMap). Local `config*.yaml` files are gitignored, except `http/config.example.yaml`.
- Helm values in `.deploy/proksi/` use camelCase (`testProbability`, `skipJsonPaths`), while the app config uses snake_case.
