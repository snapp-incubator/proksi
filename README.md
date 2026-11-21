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
| `proksi_http_mismatch_count` | counter | `type` | Main/test mismatches (`status_code`, `content_type`, `body`) |
| `proksi_http_queue_depth` | gauge | | Mirror jobs waiting in the worker queue |
