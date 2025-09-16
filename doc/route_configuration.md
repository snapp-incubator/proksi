# Route Configuration

Proksi supports flexible per-route configuration that allows you to customize comparison behavior, storage settings, and test probabilities for different API endpoints. This enables fine-grained control over how different routes are handled during shadow testing.

## Table of Contents

- [Overview](#overview)
- [Configuration Structure](#configuration-structure)
- [Route Pattern Matching](#route-pattern-matching)
- [Configuration Options](#configuration-options)
- [Examples](#examples)
- [Best Practices](#best-practices)

## Overview

The route configuration system allows you to:

- **Skip specific routes** entirely (no test upstream calls)
- **Customize comparison behavior** per route (headers, JSON paths)
- **Control storage** of request/response data
- **Adjust test probability** for different endpoints
- **Handle route parameters** with wildcard patterns

## Configuration Structure

```yaml
# Global defaults applied to all routes
global_config:
  compare_headers: true
  skip_headers: ["Date", "Server"]
  store_req_body: false
  store_resp_bodies: true
  skip_json_paths: []
  test_probability: 100

# Routes to completely skip (no test upstream call)
skip_routes:
  - "GET:/health"
  - "GET:/metrics"

# Per-route configuration overrides
route_configs:
  "POST:/api/v1/cities":
    compare_headers: false
    store_req_body: true
  "GET:/api/v1/users/*":
    skip_headers: ["Authorization"]
    test_probability: 50
```

## Route Pattern Matching

Proksi supports several types of route patterns:

### 1. Exact Match
Matches the exact HTTP method and path.

```yaml
route_configs:
  "GET:/api/v1/users":          # Matches exactly GET /api/v1/users
    store_req_body: true
  "POST:/api/v1/orders":        # Matches exactly POST /api/v1/orders
    compare_headers: false
```

### 2. Method Wildcard
Use `*` to match any HTTP method.

```yaml
route_configs:
  "*:/health":                  # Matches any method to /health
    test_probability: 0         # Never test health endpoints
  "*:/api/v1/admin/*":          # Any method to admin endpoints
    store_req_body: true
```

### 3. Trailing Wildcard
Use `/*` at the end to match any path from that point onwards.

```yaml
route_configs:
  "GET:/api/v1/users/*":        # Matches GET /api/v1/users/123, /api/v1/users/abc/profile, etc.
    skip_headers: ["Cookie"]
  "POST:/api/v2/*":             # Matches any POST to /api/v2/ and sub-paths
    store_resp_bodies: false
```

### 4. Route Parameters (Single Segment)
Use `*` to match exactly one path segment (route parameter).

```yaml
route_configs:
  "GET:/api/v1/users/*/profile":     # Matches GET /api/v1/users/123/profile
    skip_headers: ["Authorization"]   # But NOT /api/v1/users/123/456/profile
  
  "PUT:/api/v1/orders/*/status":     # Matches PUT /api/v1/orders/abc/status
    store_req_body: true             # But NOT /api/v1/orders/status
```

### 5. Multiple Route Parameters
Use multiple `*` to match multiple route parameters.

```yaml
route_configs:
  "GET:/api/v1/users/*/posts/*":     # Matches GET /api/v1/users/123/posts/456
    test_probability: 75             # But NOT /api/v1/users/123/posts
  
  "DELETE:/api/v1/shops/*/items/*":  # Matches DELETE /api/v1/shops/abc/items/def
    store_req_body: true
```

### 6. Mixed Patterns
Combine fixed segments with parameters.

```yaml
route_configs:
  "GET:/api/*/v1/users":             # Matches GET /api/public/v1/users, /api/private/v1/users
    skip_headers: ["X-API-Version"]
  
  "POST:/api/v1/*/orders/*/items":   # Matches POST /api/v1/shops/orders/123/items
    store_resp_bodies: true
```

## Configuration Options

### Global Configuration (`global_config`)

These settings apply to all routes unless overridden by specific route configurations.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `compare_headers` | boolean | `true` | Compare response headers between upstreams |
| `skip_headers` | string[] | `["Date", "Server"]` | Headers to ignore during comparison |
| `store_req_body` | boolean | `false` | Store request body when responses differ |
| `store_resp_bodies` | boolean | `true` | Store response bodies when they differ |
| `skip_json_paths` | string[] | `[]` | JSON paths to ignore during comparison |
| `test_probability` | integer | `100` | Percentage of requests to send to test upstream (0-100) |

### Route-Specific Configuration (`route_configs`)

Each route pattern can override any of the global configuration options.

### Skip Routes (`skip_routes`)

Routes listed here will bypass the test upstream entirely - no comparison or storage occurs.

```yaml
skip_routes:
  - "GET:/health"              # Health checks
  - "GET:/metrics"             # Metrics endpoint
  - "*:/static/*"              # Static assets
  - "OPTIONS:*"                # CORS preflight requests
```

## Examples

### E-commerce API Configuration

```yaml
global_config:
  compare_headers: true
  skip_headers: ["Date", "Server", "X-Request-ID"]
  store_req_body: false
  store_resp_bodies: true
  skip_json_paths: ["timestamp", "request_id"]
  test_probability: 100

skip_routes:
  - "GET:/health"
  - "GET:/metrics"
  - "*:/static/*"
  - "OPTIONS:*"

route_configs:
  # Product catalog - high traffic, reduced testing
  "GET:/api/v1/products":
    test_probability: 25
    skip_headers: ["Cache-Control", "ETag"]
  
  "GET:/api/v1/products/*":
    test_probability: 50
    skip_json_paths: ["view_count", "last_viewed"]
  
  # User management - sensitive data
  "GET:/api/v1/users/*/profile":
    skip_headers: ["Authorization", "Cookie"]
    store_req_body: false
    skip_json_paths: ["email", "phone", "address"]
  
  "PUT:/api/v1/users/*/profile":
    store_req_body: true
    skip_headers: ["Authorization"]
  
  # Order processing - critical path
  "POST:/api/v1/orders":
    store_req_body: true
    test_probability: 100
  
  "GET:/api/v1/orders/*/items":
    skip_json_paths: ["internal_price", "vendor_id"]
  
  "POST:/api/v1/orders/*/payment":
    skip_headers: ["Payment-Token", "Authorization"]
    store_req_body: true
  
  # Admin endpoints - full logging
  "*:/api/v1/admin/*":
    store_req_body: true
    store_resp_bodies: true
    test_probability: 100
  
  # Search API - performance sensitive
  "GET:/api/v1/search":
    test_probability: 10
    store_resp_bodies: false
    skip_json_paths: ["search_time", "result_count"]
```

### Multi-tenant SaaS Configuration

```yaml
global_config:
  compare_headers: true
  skip_headers: ["Date", "Server"]
  store_req_body: false
  store_resp_bodies: true
  test_probability: 100

route_configs:
  # Tenant-specific endpoints
  "GET:/api/v1/tenants/*/users":
    skip_headers: ["X-Tenant-ID"]
    test_probability: 75
  
  "POST:/api/v1/tenants/*/users/*/actions":
    store_req_body: true
    skip_headers: ["Authorization", "X-Tenant-ID"]
  
  # Workspace management
  "GET:/api/v1/workspaces/*/projects/*":
    skip_json_paths: ["created_by", "updated_by"]
  
  # Billing endpoints - sensitive
  "*:/api/v1/tenants/*/billing/*":
    skip_headers: ["Payment-Method", "Authorization"]
    store_req_body: true
    test_probability: 100
```

### Microservices Gateway Configuration

```yaml
global_config:
  compare_headers: false  # Different services may have different headers
  store_resp_bodies: true
  test_probability: 50

route_configs:
  # User service
  "*:/api/v1/users/*":
    compare_headers: true
    skip_headers: ["X-Service-Version"]
  
  # Product service
  "*:/api/v1/products/*":
    test_probability: 25  # High traffic service
    skip_json_paths: ["cache_timestamp"]
  
  # Order service - critical
  "*:/api/v1/orders/*":
    test_probability: 100
    store_req_body: true
    compare_headers: true
  
  # Notification service - async
  "POST:/api/v1/notifications":
    test_probability: 10
    store_resp_bodies: false
  
  # File service - large responses
  "GET:/api/v1/files/*":
    store_resp_bodies: false
    test_probability: 20
```

## Best Practices

### 1. Start with Global Defaults
Configure sensible global defaults and only override specific routes as needed.

```yaml
global_config:
  compare_headers: true
  skip_headers: ["Date", "Server", "X-Request-ID"]
  store_req_body: false
  store_resp_bodies: true
  test_probability: 100
```

### 2. Skip Non-Essential Endpoints
Always skip health checks, metrics, and static assets.

```yaml
skip_routes:
  - "GET:/health"
  - "GET:/metrics"
  # TODO: there is a bug for wildcard METHOD, the *:/static..
  # it's something related to helm configmap not config loader itself
  - "*:/static/*"
  - "OPTIONS:*"
```

### 3. Handle Sensitive Data
Skip or mask sensitive information in headers and response bodies.

```yaml
route_configs:
  "*:/api/v1/users/*":
    skip_headers: ["Authorization", "Cookie"]
    skip_json_paths: ["email", "phone", "ssn"]
```

### 4. Optimize High-Traffic Endpoints
Reduce test probability for high-traffic, low-risk endpoints.

```yaml
route_configs:
  "GET:/api/v1/products":
    test_probability: 25  # Only test 25% of product list requests
```

### 5. Full Testing for Critical Paths
Ensure 100% testing for critical business operations.

```yaml
route_configs:
  "POST:/api/v1/orders":
    test_probability: 100
    store_req_body: true
  "POST:/api/v1/payments":
    test_probability: 100
    store_req_body: true
```

### 6. Use Specific Patterns
Prefer specific patterns over broad wildcards for better control.

```yaml
# Good: Specific pattern
"GET:/api/v1/users/*/profile":
  skip_headers: ["Authorization"]

# Avoid: Too broad
"GET:/api/v1/users/*":
  skip_headers: ["Authorization"]  # Affects all user endpoints
```

### 7. Monitor and Adjust
Regularly review your configuration based on:
- Traffic patterns
- Error rates
- Performance impact
- Storage costs

### 8. Document Your Patterns
Comment your route configurations to explain the business logic.

```yaml
route_configs:
  # Payment processing - critical path, full testing required
  "POST:/api/v1/payments/*":
    test_probability: 100
    store_req_body: true
    
  # User avatars - non-critical, reduce testing to save resources
  "GET:/api/v1/users/*/avatar":
    test_probability: 10
    store_resp_bodies: false
```

## Pattern Matching Priority

Route patterns are evaluated in the order they appear in the configuration. More specific patterns should be placed before broader ones:

```yaml
route_configs:
  # Specific patterns first
  "GET:/api/v1/users/me/profile":      # Most specific
    store_req_body: true
    
  "GET:/api/v1/users/*/profile":       # Less specific
    skip_headers: ["Authorization"]
    
  "GET:/api/v1/users/*":               # Broad pattern
    test_probability: 50
    
  "*:/api/v1/*":                       # Broadest pattern
    compare_headers: true
```

This ensures that the most specific configuration is applied to each request.
