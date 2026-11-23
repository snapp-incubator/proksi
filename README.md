# Proksi

Proksi is a set of tools and applications that help you shadow test your web application.

**Warning: This project is in development phase and is NOT ready for production yet**

## Proksi HTTP

Proksi HTTP is an HTTP proxy server that shadow HTTP requests to a main and a test upstream and compare the responses.

### Metrics

When `metrics.enabled` is true, Prometheus metrics are served on `metrics.bind` at `/metrics`:

| Metric | Type | Labels | Description |
|---|---|---|---|
| `proksi_http_request_count` | counter | `status`, `method`, `upstream` | Requests per upstream (`main_upstream`/`test_upstream`); `status="client_error"` marks upstream errors |
| `proksi_http_request_duration` | histogram | `method`, `upstream` | Upstream request latency in seconds |
| `proksi_http_mismatch_count` | counter | `type` | Main/test mismatches (`status_code`, `headers`, `body`) |
| `proksi_http_queue_depth` | gauge | | Mirror jobs waiting in the worker queue |
| `proksi_http_comparison_results` | counter | `diff_type` | Comparison outcomes (`identical`, `status_diff`, `header_diff`, `body_diff`) |
| `proksi_http_route_skips` | counter | `skip_reason` | Requests not mirrored because the route is in `skip_routes` |
| `proksi_http_status_2xx_vs_non2xx_count` | counter | | Main returned 2xx while test returned non-2xx |

## Documentation

- **[Route Configuration Guide](doc/route_configuration.md)** - Comprehensive guide to configuring per-route behavior, including route parameter patterns, comparison settings, and best practices
