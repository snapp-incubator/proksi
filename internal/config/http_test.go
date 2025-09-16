package config

import (
	"os"
	"reflect"
	"testing"
)


func TestConfigLoaderPointerDetection(t *testing.T) {
	tests := []struct {
		name           string
		yamlContent    string
		expectedConfig HTTPConfig
		description    string
	}{
		{
			name: "Explicit true values",
			yamlContent: `
route_configs:
  "POST:/api/users":
    compare_headers: enable
    store_req_body: enable
    store_resp_bodies: enable
`,
			expectedConfig: HTTPConfig{
				RouteConfigs: map[string]RouteConfig{
					"POST:/api/users": {
						CompareHeaders:  "enable", // Semantic keyword for true
						StoreReqBody:    "enable", // Semantic keyword for true
						StoreRespBodies: "enable", // Semantic keyword for true
					},
				},
			},
			description: "YAML enable values should create string 'enable'",
		},
		{
			name: "Explicit false values",
			yamlContent: `
route_configs:
  "POST:/api/users":
    compare_headers: disable
    store_req_body: disable
    store_resp_bodies: disable
`,
			expectedConfig: HTTPConfig{
				RouteConfigs: map[string]RouteConfig{
					"POST:/api/users": {
						CompareHeaders:  "disable", // Semantic keyword for false
						StoreReqBody:    "disable", // Semantic keyword for false
						StoreRespBodies: "disable", // Semantic keyword for false
					},
				},
			},
			description: "YAML disable values should create string 'disable'",
		},
		{
			name: "Missing boolean fields (empty strings)",
			yamlContent: `
route_configs:
  "POST:/api/users":
    skip_headers: ["Authorization"]
    test_probability: 75
`,
			expectedConfig: HTTPConfig{
				RouteConfigs: map[string]RouteConfig{
					"POST:/api/users": {
						CompareHeaders:  "", // Not specified in YAML, gets empty string for inheritance
						SkipHeaders:     []string{"Authorization"},
						StoreReqBody:    "", // Not specified in YAML, gets empty string for inheritance
						StoreRespBodies: "", // Not specified in YAML, gets empty string for inheritance
						TestProbability: 75,
					},
				},
			},
			description: "Missing boolean fields should result in empty string for inheritance",
		},
		{
			name: "Mixed explicit and missing fields",
			yamlContent: `
route_configs:
  "POST:/api/users":
    compare_headers: enable
    skip_headers: ["X-Request-ID"]
    store_resp_bodies: disable
    test_probability: 50
`,
			expectedConfig: HTTPConfig{
				RouteConfigs: map[string]RouteConfig{
					"POST:/api/users": {
						CompareHeaders:  "enable", // Explicitly set to enable
						SkipHeaders:     []string{"X-Request-ID"},
						StoreReqBody:    "",        // Not specified, gets "" (inherit)
						StoreRespBodies: "disable", // Explicitly set to disable
						TestProbability: 50,
					},
				},
			},
			description: "Should distinguish between explicit 'disable' and missing fields ('')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file with the YAML content
			tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer func() {
				if err := os.Remove(tmpFile.Name()); err != nil {
					t.Logf("Failed to remove temp file: %v", err)
				}
			}()

			_, err = tmpFile.WriteString(tt.yamlContent)
			if err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			// Load the configuration
			config := LoadHTTP(tmpFile.Name())

			// Check each route config
			for routePattern, expectedRouteConfig := range tt.expectedConfig.RouteConfigs {
				actualRouteConfig, exists := config.RouteConfigs[routePattern]
				if !exists {
					t.Errorf("Route config for %s not found", routePattern)
					continue
				}

				// Test CompareHeaders string
				if actualRouteConfig.CompareHeaders != expectedRouteConfig.CompareHeaders {
					t.Errorf("CompareHeaders: expected %q, got %q",
						expectedRouteConfig.CompareHeaders, actualRouteConfig.CompareHeaders)
				}

				// Test StoreReqBody string
				if actualRouteConfig.StoreReqBody != expectedRouteConfig.StoreReqBody {
					t.Errorf("StoreReqBody: expected %q, got %q",
						expectedRouteConfig.StoreReqBody, actualRouteConfig.StoreReqBody)
				}

				// Test StoreRespBodies string
				if actualRouteConfig.StoreRespBodies != expectedRouteConfig.StoreRespBodies {
					t.Errorf("StoreRespBodies: expected %q, got %q",
						expectedRouteConfig.StoreRespBodies, actualRouteConfig.StoreRespBodies)
				}

				// Test non-pointer fields
				if !reflect.DeepEqual(actualRouteConfig.SkipHeaders, expectedRouteConfig.SkipHeaders) {
					t.Errorf("SkipHeaders: expected %v, got %v",
						expectedRouteConfig.SkipHeaders, actualRouteConfig.SkipHeaders)
				}

				if actualRouteConfig.TestProbability != expectedRouteConfig.TestProbability {
					t.Errorf("TestProbability: expected %v, got %v",
						expectedRouteConfig.TestProbability, actualRouteConfig.TestProbability)
				}
			}

			t.Logf("✓ %s", tt.description)
		})
	}
}

func TestConfigLoaderWithDefaultConfigs(t *testing.T) {
	tests := []struct {
		name           string
		defaultConfig  string
		expectedGlobal GlobalConfig
		expectedRoutes map[string]RouteConfig
		description    string
	}{
		{
			name: "Complete configuration with defaults",
			defaultConfig: `
bind: "0.0.0.0:8080"
storage_type: "stdout"

global_config:
  compare_headers: true
  skip_headers: ["Date", "Server"]
  store_req_body: false
  store_resp_bodies: true
  skip_json_paths: ["timestamp"]
  test_probability: 100

skip_routes:
  - "GET:/health"
  - "*:/metrics"

route_configs:
  "POST:/api/users":
    compare_headers: disable
    store_req_body: enable
    skip_headers: ["Authorization"]
    test_probability: 75
    
  "GET:/api/orders/*/items":
    store_resp_bodies: disable
    skip_json_paths: ["internal_id"]
    test_probability: 50
    
  "PUT:/api/products/*":
    skip_headers: ["X-Version"]
`,
			expectedGlobal: GlobalConfig{
				CompareHeaders:  true,
				SkipHeaders:     []string{"Date", "Server"},
				StoreReqBody:    false,
				StoreRespBodies: true,
				SkipJSONPaths:   []string{"timestamp"},
				TestProbability: 100,
			},
			expectedRoutes: map[string]RouteConfig{
				"POST:/api/users": {
					CompareHeaders:  "disable", // Explicitly set to disable
					StoreReqBody:    "enable",  // Explicitly set to enable
					StoreRespBodies: "",        // Not specified, should inherit
					SkipHeaders:     []string{"Authorization"},
					TestProbability: 75,
				},
				"GET:/api/orders/*/items": {
					CompareHeaders:  "",        // Not specified, should inherit
					StoreReqBody:    "",        // Not specified, should inherit
					StoreRespBodies: "disable", // Explicitly set to disable
					SkipJSONPaths:   []string{"internal_id"},
					TestProbability: 50,
				},
				"PUT:/api/products/*": {
					CompareHeaders:  "", // Not specified, should inherit
					StoreReqBody:    "", // Not specified, should inherit
					StoreRespBodies: "", // Not specified, should inherit
					SkipHeaders:     []string{"X-Version"},
					TestProbability: 0, // Not specified, should be 0 (inherit via test_probability logic)
				},
			},
			description: "Complete config should correctly parse global and route-specific settings",
		},
		{
			name: "Minimal configuration with inheritance",
			defaultConfig: `
storage_type: "elasticsearch"

global_config:
  compare_headers: false
  store_req_body: true
  test_probability: 50

route_configs:
  "POST:/api/critical":
    compare_headers: enable
    test_probability: 100
    
  "GET:/api/cache/*":
    store_req_body: disable
`,
			expectedGlobal: GlobalConfig{
				CompareHeaders:  false, // Explicitly set to false in YAML
				StoreReqBody:    true,  // Explicitly set to true in YAML
				StoreRespBodies: true,  // Default value from defaultHTTP
				TestProbability: 50,    // Explicitly set to 50 in YAML
			},
			expectedRoutes: map[string]RouteConfig{
				"POST:/api/critical": {
					CompareHeaders:  "enable", // Override global false
					StoreReqBody:    "",       // Inherit global true
					StoreRespBodies: "",       // Inherit global
					TestProbability: 100,      // Override global 50
				},
				"GET:/api/cache/*": {
					CompareHeaders:  "",        // Inherit global false
					StoreReqBody:    "disable", // Override global true
					StoreRespBodies: "",        // Inherit global
					TestProbability: 0,         // Not specified
				},
			},
			description: "Should correctly inherit global settings when route fields not specified",
		},
		{
			name: "Route parameters with mixed settings",
			defaultConfig: `
storage_type: "stdout"

global_config:
  compare_headers: true
  store_resp_bodies: true
  test_probability: 100

route_configs:
  "GET:/api/v1/users/*/profile":
    compare_headers: disable
    store_req_body: enable
    
  "POST:/api/v1/shops/*/orders/*":
    store_resp_bodies: disable
    skip_headers: ["X-Shop-ID", "Authorization"]
    skip_json_paths: ["internal.shop_id", "payment.token"]
    
  "DELETE:/api/v1/orders/*/items/*":
    test_probability: 25
`,
			expectedGlobal: GlobalConfig{
				CompareHeaders:  true,
				StoreRespBodies: true,
				TestProbability: 100,
			},
			expectedRoutes: map[string]RouteConfig{
				"GET:/api/v1/users/*/profile": {
					CompareHeaders:  "disable", // Override global
					StoreReqBody:    "enable",  // Not in global, explicitly set
					StoreRespBodies: "",        // Inherit global true
					TestProbability: 0,         // Not specified
				},
				"POST:/api/v1/shops/*/orders/*": {
					CompareHeaders:  "",        // Inherit global true
					StoreReqBody:    "",        // Inherit global default (false)
					StoreRespBodies: "disable", // Override global true
					SkipHeaders:     []string{"X-Shop-ID", "Authorization"},
					SkipJSONPaths:   []string{"internal.shop_id", "payment.token"},
					TestProbability: 0, // Not specified
				},
				"DELETE:/api/v1/orders/*/items/*": {
					CompareHeaders:  "", // Inherit global true
					StoreReqBody:    "", // Inherit global default (false)
					StoreRespBodies: "", // Inherit global true
					TestProbability: 25, // Override global 100
				},
			},
			description: "Route parameters patterns should work with pointer-based inheritance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file with the YAML content
			tmpFile, err := os.CreateTemp("", "config_default_test_*.yaml")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer func() {
				if err := os.Remove(tmpFile.Name()); err != nil {
					t.Logf("Failed to remove temp file: %v", err)
				}
			}()

			_, err = tmpFile.WriteString(tt.defaultConfig)
			if err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			// Load the configuration
			config := LoadHTTP(tmpFile.Name())

			// Verify global config
			if config.GlobalConfig.CompareHeaders != tt.expectedGlobal.CompareHeaders {
				t.Errorf("Global CompareHeaders: expected %v, got %v",
					tt.expectedGlobal.CompareHeaders, config.GlobalConfig.CompareHeaders)
			}
			if config.GlobalConfig.StoreReqBody != tt.expectedGlobal.StoreReqBody {
				t.Errorf("Global StoreReqBody: expected %v, got %v",
					tt.expectedGlobal.StoreReqBody, config.GlobalConfig.StoreReqBody)
			}
			if config.GlobalConfig.StoreRespBodies != tt.expectedGlobal.StoreRespBodies {
				t.Errorf("Global StoreRespBodies: expected %v, got %v",
					tt.expectedGlobal.StoreRespBodies, config.GlobalConfig.StoreRespBodies)
			}

			// Verify route configs
			for routePattern, expectedRouteConfig := range tt.expectedRoutes {
				actualRouteConfig, exists := config.RouteConfigs[routePattern]
				if !exists {
					t.Errorf("Route config for %s not found", routePattern)
					continue
				}

				// Check string fields
				if actualRouteConfig.CompareHeaders != expectedRouteConfig.CompareHeaders {
					t.Errorf("%s CompareHeaders: expected %q, got %q",
						routePattern, expectedRouteConfig.CompareHeaders, actualRouteConfig.CompareHeaders)
				}
				if actualRouteConfig.StoreReqBody != expectedRouteConfig.StoreReqBody {
					t.Errorf("%s StoreReqBody: expected %q, got %q",
						routePattern, expectedRouteConfig.StoreReqBody, actualRouteConfig.StoreReqBody)
				}
				if actualRouteConfig.StoreRespBodies != expectedRouteConfig.StoreRespBodies {
					t.Errorf("%s StoreRespBodies: expected %q, got %q",
						routePattern, expectedRouteConfig.StoreRespBodies, actualRouteConfig.StoreRespBodies)
				}

				// Check non-pointer fields
				if !reflect.DeepEqual(actualRouteConfig.SkipHeaders, expectedRouteConfig.SkipHeaders) {
					t.Errorf("%s SkipHeaders: expected %v, got %v",
						routePattern, expectedRouteConfig.SkipHeaders, actualRouteConfig.SkipHeaders)
				}
				if !reflect.DeepEqual(actualRouteConfig.SkipJSONPaths, expectedRouteConfig.SkipJSONPaths) {
					t.Errorf("%s SkipJSONPaths: expected %v, got %v",
						routePattern, expectedRouteConfig.SkipJSONPaths, actualRouteConfig.SkipJSONPaths)
				}
				if actualRouteConfig.TestProbability != expectedRouteConfig.TestProbability {
					t.Errorf("%s TestProbability: expected %v, got %v",
						routePattern, expectedRouteConfig.TestProbability, actualRouteConfig.TestProbability)
				}
			}

			t.Logf("✓ %s", tt.description)
		})
	}
}

func TestFormatRoute(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		expected string
	}{
		{"Basic route", "GET", "/api/users", "GET:/api/users"},
		{"Lowercase method", "get", "/api/users", "GET:/api/users"},
		{"Mixed case method", "Post", "/api/orders", "POST:/api/orders"},
		{"Root path", "GET", "/", "GET:/"},
		{"Complex path", "PUT", "/api/v1/users/123/profile", "PUT:/api/v1/users/123/profile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatRoute(tt.method, tt.path)
			if result != tt.expected {
				t.Errorf("FormatRoute(%q, %q) = %q, want %q", tt.method, tt.path, result, tt.expected)
			}
		})
	}
}

func TestParseRoute(t *testing.T) {
	tests := []struct {
		name           string
		route          string
		expectedMethod string
		expectedPath   string
	}{
		{"Basic route", "GET:/api/users", "GET", "/api/users"},
		{"Root path", "POST:/", "POST", "/"},
		{"Complex path", "PUT:/api/v1/users/123", "PUT", "/api/v1/users/123"},
		{"No method (wildcard)", "/api/users", "*", "/api/users"},
		{"Empty route", "", "*", ""},
		{"Only method", "GET:", "GET", ""},
		{"Multiple colons", "GET:/api:test", "GET", "/api:test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, path := ParseRoute(tt.route)
			if method != tt.expectedMethod || path != tt.expectedPath {
				t.Errorf("ParseRoute(%q) = (%q, %q), want (%q, %q)",
					tt.route, method, path, tt.expectedMethod, tt.expectedPath)
			}
		})
	}
}

func TestMatchRoute(t *testing.T) {
	tests := []struct {
		name         string
		requestRoute string
		configRoute  string
		expected     bool
	}{
		// Exact matches
		{"Exact match", "GET:/api/users", "GET:/api/users", true},
		{"Method mismatch", "POST:/api/users", "GET:/api/users", false},
		{"Path mismatch", "GET:/api/orders", "GET:/api/users", false},

		// Method wildcards
		{"Method wildcard match", "GET:/health", "*:/health", true},
		{"Method wildcard different methods", "POST:/health", "*:/health", true},

		// Trailing wildcards
		{"Trailing wildcard match", "GET:/api/users/123", "GET:/api/users/*", true},
		{"Trailing wildcard deep match", "GET:/api/users/123/profile/settings", "GET:/api/users/*", true},
		{"Trailing wildcard no match", "GET:/api/orders", "GET:/api/users/*", false},

		// Single segment parameters
		{"Single parameter match", "GET:/api/users/123/profile", "GET:/api/users/*/profile", true},
		{"Single parameter string", "GET:/api/users/abc/profile", "GET:/api/users/*/profile", true},
		{"Single parameter mismatch - missing segment", "GET:/api/users/profile", "GET:/api/users/*/profile", false},
		{"Single parameter mismatch - extra segment", "GET:/api/users/123/456/profile", "GET:/api/users/*/profile", false},

		// Multiple parameters
		{"Multiple parameters", "GET:/api/users/123/posts/456", "GET:/api/users/*/posts/*", true},
		{"Multiple parameters strings", "GET:/api/users/abc/posts/def", "GET:/api/users/*/posts/*", true},
		{"Multiple parameters missing last", "GET:/api/users/123/posts", "GET:/api/users/*/posts/*", false},
		{"Multiple parameters extra segments", "GET:/api/users/123/posts/456/comments", "GET:/api/users/*/posts/*", false},

		// Mixed patterns
		{"Mixed wildcards with fixed segments", "GET:/api/public/v1/users", "GET:/api/*/v1/users", true},
		{"Mixed wildcards mismatch", "GET:/api/public/v2/users", "GET:/api/*/v1/users", false},

		// Root level parameters
		{"Root parameter", "GET:/anything", "GET:/*", true},
		{"Two root parameters", "GET:/api/users", "GET:/*/*", true},
		{"Two root parameters mismatch", "GET:/api", "GET:/*/*", false},

		// Edge cases
		{"Empty paths", "GET:", "GET:", true},
		{"Single wildcard", "GET:/test", "GET:*", true}, // Actually matches via Go path.Match
		{"Root wildcard match", "GET:/", "GET:/*", true},

		// Go path.Match fallback patterns
		{"Character class match", "GET:/users/123", "GET:/users/[0-9]*", false}, // Our implementation doesn't use path.Match for patterns with *
		{"Character class no match", "GET:/users/abc", "GET:/users/[0-9]*", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchRoute(tt.requestRoute, tt.configRoute)
			if result != tt.expected {
				t.Errorf("MatchRoute(%q, %q) = %t, want %t",
					tt.requestRoute, tt.configRoute, result, tt.expected)
			}
		})
	}
}

func TestMatchPath(t *testing.T) {
	tests := []struct {
		name        string
		requestPath string
		configPath  string
		expected    bool
	}{
		// Exact matches
		{"Exact match", "/api/users", "/api/users", true},
		{"Exact mismatch", "/api/users", "/api/orders", false},

		// No wildcards (Go path.Match)
		{"No wildcard exact", "/users/123", "/users/123", true},
		{"Character class", "/users/123", "/users/[0-9]*", false}, // Our implementation routes * patterns to segment matching
		{"Character class no match", "/users/abc", "/users/[0-9]*", false},

		// Single segment wildcards
		{"Single segment wildcard", "/api/users/123/profile", "/api/users/*/profile", true},
		{"Single segment wildcard mismatch segments", "/api/users/profile", "/api/users/*/profile", false},

		// Multiple segment wildcards
		{"Multiple segments", "/api/users/123/posts/456", "/api/users/*/posts/*", true},
		{"Multiple segments root", "/api/users", "/*/*", true},

		// Trailing wildcards vs segment wildcards
		{"True trailing wildcard", "/api/users/123/anything/else", "/api/users/*", true},
		{"Segment wildcard not trailing", "/api/users/123", "/api/users/*", true}, // This is ambiguous in current implementation

		// Edge cases
		{"Empty request path", "", "", true},
		{"Root paths", "/", "/", true},
		{"Root wildcard", "/anything", "/*", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchPath(tt.requestPath, tt.configPath)
			if result != tt.expected {
				t.Errorf("matchPath(%q, %q) = %t, want %t",
					tt.requestPath, tt.configPath, result, tt.expected)
			}
		})
	}
}

func TestMatchSegmentWildcards(t *testing.T) {
	tests := []struct {
		name        string
		requestPath string
		configPath  string
		expected    bool
	}{
		// Trailing wildcards (should use prefix matching)
		{"Pure trailing wildcard", "/api/users/123/anything", "/api/users/*", true},
		{"Pure trailing wildcard no match", "/api/orders/123", "/api/users/*", false},
		{"Root trailing wildcard", "/anything/here", "/*", true},

		// Segment-by-segment matching (patterns with other wildcards)
		{"Mixed pattern not trailing", "/api/test/v1/users", "/api/*/v1/*", true},
		{"Multiple parameters", "/api/users/123/posts/456", "/api/users/*/posts/*", true},
		{"Root segments", "/api/users", "/*/*", true},
		{"Root segments mismatch count", "/api", "/*/*", false},

		// Edge cases
		{"Empty paths", "", "", true},
		{"Single segment match", "/test", "/*", true},
		{"Single parameter", "/api", "/*", true},

		// Segment count validation
		{"Too few segments", "/api", "/api/*/test", false},
		{"Too many segments", "/api/test/extra/stuff", "/api/*/test", false},
		{"Perfect segment match", "/api/test/endpoint", "/api/*/endpoint", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchSegmentWildcards(tt.requestPath, tt.configPath)
			if result != tt.expected {
				t.Errorf("matchSegmentWildcards(%q, %q) = %t, want %t",
					tt.requestPath, tt.configPath, result, tt.expected)
			}
		})
	}
}

func TestIsValidRoutePattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected bool
	}{
		// Valid patterns
		{"Simple path", "/api/users", true},
		{"Root path", "/", true},
		{"With parameters", "/api/users/*/profile", true},
		{"Multiple parameters", "/api/*/users/*/posts", true},
		{"Trailing wildcard", "/api/users/*", true},
		{"Single wildcard", "*", true},
		{"Root parameter", "/*", true},
		{"Multiple root parameters", "/*/*", true},

		// Invalid patterns
		{"Empty path", "", false},
		{"No leading slash", "api/users", false},
		{"Double wildcards", "/api/**/users", false},
		{"Invalid trailing wildcard", "/api/users*", false},
		{"Invalid trailing pattern", "/api/test*", false},

		// Edge cases
		{"Just slash and wildcard", "/*", true},
		{"Multiple slashes", "/api//users", true}, // This might be debatable
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidRoutePattern(tt.pattern)
			if result != tt.expected {
				t.Errorf("isValidRoutePattern(%q) = %t, want %t", tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestHTTPConfig_migrateFromLegacyConfig(t *testing.T) {
	tests := []struct {
		name           string
		config         HTTPConfig
		expectedGlobal GlobalConfig
	}{
		{
			name: "Migrate all legacy fields",
			config: HTTPConfig{
				CompareHeaders:     false,
				TestProbability:    75,
				LogResponsePayload: false,
				SkipJSONPaths:      []string{"timestamp", "id"},
				GlobalConfig: GlobalConfig{
					CompareHeaders:  true,       // Should be overridden
					TestProbability: 100,        // Should be overridden
					StoreRespBodies: true,       // Should be overridden
					SkipJSONPaths:   []string{}, // Should be overridden
				},
			},
			expectedGlobal: GlobalConfig{
				CompareHeaders:  false,                       // From legacy
				TestProbability: 75,                          // From legacy
				StoreRespBodies: false,                       // From legacy LogResponsePayload
				SkipJSONPaths:   []string{"timestamp", "id"}, // From legacy
			},
		},
		{
			name: "No migration when legacy fields match defaults",
			config: HTTPConfig{
				CompareHeaders:     true,       // Matches global default
				TestProbability:    0,          // Zero value, no migration
				LogResponsePayload: true,       // Matches global default
				SkipJSONPaths:      []string{}, // Empty, no migration
				GlobalConfig: GlobalConfig{
					CompareHeaders:  true,
					TestProbability: 100,
					StoreRespBodies: true,
					SkipJSONPaths:   []string{},
				},
			},
			expectedGlobal: GlobalConfig{
				CompareHeaders:  true,       // No change
				TestProbability: 100,        // No change
				StoreRespBodies: true,       // No change
				SkipJSONPaths:   []string{}, // No change
			},
		},
		{
			name: "Partial migration",
			config: HTTPConfig{
				CompareHeaders:     false,            // Different from global, should migrate
				TestProbability:    50,               // Non-zero, different from global, should migrate
				LogResponsePayload: true,             // Same as global StoreRespBodies, no migration
				SkipJSONPaths:      []string{"test"}, // Non-empty, global empty, should migrate
				GlobalConfig: GlobalConfig{
					CompareHeaders:  true,
					TestProbability: 100,
					StoreRespBodies: true,
					SkipJSONPaths:   []string{},
				},
			},
			expectedGlobal: GlobalConfig{
				CompareHeaders:  false,            // Migrated
				TestProbability: 50,               // Migrated
				StoreRespBodies: true,             // No migration (same value)
				SkipJSONPaths:   []string{"test"}, // Migrated
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.migrateFromLegacyConfig()

			if tt.config.GlobalConfig.CompareHeaders != tt.expectedGlobal.CompareHeaders {
				t.Errorf("CompareHeaders = %t, want %t",
					tt.config.GlobalConfig.CompareHeaders, tt.expectedGlobal.CompareHeaders)
			}
			if tt.config.GlobalConfig.TestProbability != tt.expectedGlobal.TestProbability {
				t.Errorf("TestProbability = %d, want %d",
					tt.config.GlobalConfig.TestProbability, tt.expectedGlobal.TestProbability)
			}
			if tt.config.GlobalConfig.StoreRespBodies != tt.expectedGlobal.StoreRespBodies {
				t.Errorf("StoreRespBodies = %t, want %t",
					tt.config.GlobalConfig.StoreRespBodies, tt.expectedGlobal.StoreRespBodies)
			}
			if !reflect.DeepEqual(tt.config.GlobalConfig.SkipJSONPaths, tt.expectedGlobal.SkipJSONPaths) {
				t.Errorf("SkipJSONPaths = %v, want %v",
					tt.config.GlobalConfig.SkipJSONPaths, tt.expectedGlobal.SkipJSONPaths)
			}
		})
	}
}

func TestHTTPConfig_PrecomputeRouteConfigs(t *testing.T) {
	config := HTTPConfig{
		GlobalConfig: GlobalConfig{
			CompareHeaders:  true,
			SkipHeaders:     []string{"Date", "Server"},
			StoreReqBody:    false,
			StoreRespBodies: true,
			SkipJSONPaths:   []string{"timestamp"},
			TestProbability: 100,
		},
		SkipRoutes: []string{
			"GET:/health",
			"*:/metrics",
		},
		RouteConfigs: map[string]RouteConfig{
			"POST:/api/users": {
				CompareHeaders:  "disable",
				SkipHeaders:     []string{"Authorization"},
				StoreReqBody:    "enable",
				StoreRespBodies: "enable",
				SkipJSONPaths:   []string{"password"},
				TestProbability: 75,
			},
			"GET:/api/orders/*": {
				CompareHeaders:  "", // Inherit from global
				SkipHeaders:     []string{"Cookie"},
				StoreReqBody:    "", // Inherit from global
				StoreRespBodies: "", // Inherit from global
				SkipJSONPaths:   []string{"internal_id"},
				TestProbability: 50,
			},
		},
	}

	computed := config.PrecomputeRouteConfigs()

	// Test global config
	expectedGlobal := ComputedRouteConfig{
		CompareHeaders:  true,
		SkipHeaders:     []string{"Date", "Server"},
		StoreReqBody:    false,
		StoreRespBodies: true,
		SkipJSONPaths:   []string{"timestamp"},
		TestProbability: 100,
	}

	if !reflect.DeepEqual(computed.Global, expectedGlobal) {
		t.Errorf("Global config mismatch.\nGot:  %+v\nWant: %+v", computed.Global, expectedGlobal)
	}

	// Test skip routes
	expectedSkipRoutes := map[string]bool{
		"GET:/health": true,
		"*:/metrics":  true,
	}
	if !reflect.DeepEqual(computed.SkipRoutes, expectedSkipRoutes) {
		t.Errorf("Skip routes mismatch.\nGot:  %+v\nWant: %+v", computed.SkipRoutes, expectedSkipRoutes)
	}

	// Test route-specific configs
	// POST:/api/users should override all fields
	expectedPostUsers := ComputedRouteConfig{
		CompareHeaders:  false,                                       // Overridden
		SkipHeaders:     []string{"Date", "Server", "Authorization"}, // Merged
		StoreReqBody:    true,                                        // Overridden
		StoreRespBodies: true,                                        // Overridden
		SkipJSONPaths:   []string{"timestamp", "password"},           // Merged
		TestProbability: 75,                                          // Overridden
	}

	if gotConfig, exists := computed.Routes["POST:/api/users"]; !exists {
		t.Error("POST:/api/users config not found")
	} else {
		if !reflect.DeepEqual(gotConfig, expectedPostUsers) {
			t.Errorf("POST:/api/users config mismatch.\nGot:  %+v\nWant: %+v", gotConfig, expectedPostUsers)
		}
	}

	// GET:/api/orders/* should inherit some fields and override others
	expectedGetOrders := ComputedRouteConfig{
		CompareHeaders:  true,                                 // From global (inherited via nil pointer)
		SkipHeaders:     []string{"Date", "Server", "Cookie"}, // Merged
		StoreReqBody:    false,                                // From global (inherited via nil pointer)
		StoreRespBodies: true,                                 // From global (inherited via nil pointer)
		SkipJSONPaths:   []string{"timestamp", "internal_id"}, // Merged
		TestProbability: 50,                                   // Overridden
	}

	if gotConfig, exists := computed.Routes["GET:/api/orders/*"]; !exists {
		t.Error("GET:/api/orders/* config not found")
	} else {
		if !reflect.DeepEqual(gotConfig, expectedGetOrders) {
			t.Errorf("GET:/api/orders/* config mismatch.\nGot:  %+v\nWant: %+v", gotConfig, expectedGetOrders)
		}
	}
}

func TestGetRouteConfig(t *testing.T) {
	// Set up ComputedConfigs for testing
	ComputedConfigs = &ComputedRouteConfigs{
		Global: ComputedRouteConfig{
			CompareHeaders:  true,
			SkipHeaders:     []string{"Date"},
			StoreReqBody:    false,
			StoreRespBodies: true,
			TestProbability: 100,
		},
		Routes: map[string]ComputedRouteConfig{
			"POST:/api/users": {
				CompareHeaders:  false,
				SkipHeaders:     []string{"Authorization"},
				StoreReqBody:    true,
				StoreRespBodies: true,
				TestProbability: 75,
			},
			"POST:/api/v1/services/*/items": {
				CompareHeaders:  true,
				SkipHeaders:     []string{"X-Service"},
				StoreReqBody:    false,
				StoreRespBodies: false,
				TestProbability: 50,
			},
		},
		SkipRoutes: map[string]bool{
			"GET:/health": true,
		},
	}

	tests := []struct {
		name     string
		route    string
		expected ComputedRouteConfig
	}{
		{
			name:  "Existing specific route config",
			route: "POST:/api/users",
			expected: ComputedRouteConfig{
				CompareHeaders:  false,
				SkipHeaders:     []string{"Authorization"},
				StoreReqBody:    true,
				StoreRespBodies: true,
				TestProbability: 75,
			},
		},
		{
			name:  "Non-existing route returns global config",
			route: "GET:/api/orders",
			expected: ComputedRouteConfig{
				CompareHeaders:  true,
				SkipHeaders:     []string{"Date"},
				StoreReqBody:    false,
				StoreRespBodies: true,
				TestProbability: 100,
			},
		},
		{
			name:  "Route parameter matching",
			route: "POST:/api/v1/services/payment/items",
			expected: ComputedRouteConfig{
				CompareHeaders:  true,
				SkipHeaders:     []string{"X-Service"},
				StoreReqBody:    false,
				StoreRespBodies: false,
				TestProbability: 50,
			},
		},
		{
			name:  "Route parameter matching - different service",
			route: "POST:/api/v1/services/billing/items",
			expected: ComputedRouteConfig{
				CompareHeaders:  true,
				SkipHeaders:     []string{"X-Service"},
				StoreReqBody:    false,
				StoreRespBodies: false,
				TestProbability: 50,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRouteConfig(tt.route)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("GetRouteConfig(%q) = %+v, want %+v", tt.route, result, tt.expected)
			}
		})
	}
}

func TestIsRouteSkipped(t *testing.T) {
	// Set up ComputedConfigs for testing
	ComputedConfigs = &ComputedRouteConfigs{
		SkipRoutes: map[string]bool{
			"GET:/health": true,
			"*:/metrics":  true,
			"POST:/debug": true,
		},
	}

	tests := []struct {
		name     string
		route    string
		expected bool
	}{
		{"Skipped route", "GET:/health", true},
		{"Skipped with wildcard method - exact match", "*:/metrics", true},
		{"Skipped with wildcard method - pattern match GET", "GET:/metrics", true},
		{"Skipped with wildcard method - pattern match POST", "POST:/metrics", true},
		{"Another skipped route", "POST:/debug", true},
		{"Non-skipped route", "GET:/api/users", false},
		{"Non-existing route", "DELETE:/api/orders", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRouteSkipped(tt.route)
			if result != tt.expected {
				t.Errorf("IsRouteSkipped(%q) = %t, want %t", tt.route, result, tt.expected)
			}
		})
	}
}

// Benchmark tests for performance validation
func BenchmarkMatchRoute(b *testing.B) {
	testCases := []struct {
		name         string
		requestRoute string
		configRoute  string
	}{
		{"Exact match", "GET:/api/users", "GET:/api/users"},
		{"Single parameter", "GET:/api/users/123/profile", "GET:/api/users/*/profile"},
		{"Multiple parameters", "GET:/api/users/123/posts/456", "GET:/api/users/*/posts/*"},
		{"Trailing wildcard", "GET:/api/users/123/profile/settings", "GET:/api/users/*"},
		{"Method wildcard", "POST:/health", "*:/health"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				MatchRoute(tc.requestRoute, tc.configRoute)
			}
		})
	}
}

func BenchmarkGetRouteConfig(b *testing.B) {
	// Set up realistic ComputedConfigs
	ComputedConfigs = &ComputedRouteConfigs{
		Global: ComputedRouteConfig{
			CompareHeaders:  true,
			TestProbability: 100,
		},
		Routes: map[string]ComputedRouteConfig{
			"POST:/api/users":   {TestProbability: 75},
			"GET:/api/orders/*": {TestProbability: 50},
			"PUT:/api/products": {TestProbability: 90},
		},
	}

	routes := []string{
		"POST:/api/users",
		"GET:/api/orders/123",
		"PUT:/api/products",
		"DELETE:/api/unknown",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		route := routes[i%len(routes)]
		GetRouteConfig(route)
	}
}

func BenchmarkIsRouteSkipped(b *testing.B) {
	// Set up realistic ComputedConfigs
	ComputedConfigs = &ComputedRouteConfigs{
		SkipRoutes: map[string]bool{
			"GET:/health":    true,
			"GET:/metrics":   true,
			"*:/static/*":    true,
			"OPTIONS:/api/*": true,
		},
	}

	routes := []string{
		"GET:/health",
		"GET:/metrics",
		"*:/static/css",
		"OPTIONS:/api/users",
		"POST:/api/users",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		route := routes[i%len(routes)]
		IsRouteSkipped(route)
	}
}
