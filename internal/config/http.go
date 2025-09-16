package config

import (
	"fmt"
	"path"
	"strings"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"go.uber.org/zap"

	"github.com/snapp-incubator/proksi/internal/logging"
)

var (

	// HTTP is the config for Proksi HTTP
	HTTP *HTTPConfig

	// ComputedConfigs contains pre-computed route configurations for fast runtime lookup
	ComputedConfigs *ComputedRouteConfigs
)

var defaultHTTP = HTTPConfig{
	Bind:     "0.0.0.0:9090",
	LogLevel: "warn",
	Metrics: metric{
		Enabled: true,
		Bind:    "0.0.0.0:9001",
	},
	StorageType: "stdout",
	Elasticsearch: Elasticsearch{
		Addresses:              []string{"::9200"},
		Username:               "",
		Password:               "",
		CloudID:                "",
		APIKey:                 "",
		ServiceToken:           "",
		CertificateFingerprint: "",
	},
	Upstreams: struct {
		Main httpUpstream `koanf:"main"`
		Test httpUpstream `koanf:"test"`
	}{
		Main: httpUpstream{Address: "127.0.0.1:8080"},
		Test: httpUpstream{Address: "127.0.0.1:8081"},
	},
	Worker: worker{
		Count:     50,
		QueueSize: 2048,
	},

	// New per-route configuration defaults
	GlobalConfig: GlobalConfig{
		CompareHeaders:  true,
		SkipHeaders:     []string{},
		StoreReqBody:    false,
		StoreRespBodies: true,
		SkipJSONPaths:   []string{},
		TestProbability: 100,
	},
	RouteConfigs: make(map[string]RouteConfig),
	SkipRoutes:   []string{},

	// Legacy fields for backward compatibility
	SkipJSONPaths:      []string{},
	TestProbability:    100,
	LogResponsePayload: true,
	CompareHeaders:     true,
}

// HTTPConfig represent config of the Proksi HTTP.
type HTTPConfig struct {
	Bind          string        `koanf:"bind"`
	LogLevel      string        `koanf:"log_level"` // Log level: "debug", "info", "warn", "error", "fatal"
	Metrics       metric        `koanf:"metrics"`
	StorageType   string        `koanf:"storage_type"` // Storage backend type: "elasticsearch" or "stdout"
	Elasticsearch Elasticsearch `koanf:"elasticsearch"`
	Upstreams     struct {
		Main httpUpstream `koanf:"main"`
		Test httpUpstream `koanf:"test"`
	} `koanf:"upstreams"`
	Worker worker `koanf:"worker"`

	// New per-route configuration
	GlobalConfig GlobalConfig           `koanf:"global_config"`
	RouteConfigs map[string]RouteConfig `koanf:"route_configs"`
	SkipRoutes   []string               `koanf:"skip_routes"`

	// Legacy fields for backward compatibility - deprecated but still supported
	SkipJSONPaths      []string `koanf:"skip_json_paths"`      // Deprecated: use GlobalConfig.SkipJSONPaths
	TestProbability    uint64   `koanf:"test_probability"`     // Deprecated: use GlobalConfig.TestProbability
	LogResponsePayload bool     `koanf:"log_response_payload"` // Deprecated: use GlobalConfig.StoreRespBodies
	CompareHeaders     bool     `koanf:"compare_headers"`      // Deprecated: use GlobalConfig.CompareHeaders
}

type httpUpstream struct {
	Address string `koanf:"address"`
}

type worker struct {
	Count     uint `koanf:"count"`
	QueueSize uint `koanf:"queue_size"`
}

// RouteConfig represents per-route configuration overrides
type RouteConfig struct {
	CompareHeaders  string   `koanf:"compare_headers"`   // Override global compare headers setting ("" = inherit, "enable"/"disable" = override)
	CompareBody     string   `koanf:"compare_body"`      // Override global compare body setting ("" = inherit, "enable"/"disable" = override)
	SkipHeaders     []string `koanf:"skip_headers"`      // Headers to skip during comparison
	StoreReqBody    string   `koanf:"store_req_body"`    // Store request body on differences ("" = inherit, "enable"/"disable" = override)
	StoreRespBodies string   `koanf:"store_resp_bodies"` // Store response bodies on differences ("" = inherit, "enable"/"disable" = override)
	SkipJSONPaths   []string `koanf:"skip_json_paths"`   // Route-specific JSON paths to skip
	TestProbability uint64   `koanf:"test_probability"`  // Override global test probability for this route (0 = inherit)
}

// GlobalConfig represents global default configuration
type GlobalConfig struct {
	CompareHeaders  bool     `koanf:"compare_headers"`   // Default: true
	CompareBody     bool     `koanf:"compare_body"`      // Default: true
	SkipHeaders     []string `koanf:"skip_headers"`      // Global headers to skip
	StoreReqBody    bool     `koanf:"store_req_body"`    // Default: false
	StoreRespBodies bool     `koanf:"store_resp_bodies"` // Default: true (current LogResponsePayload)
	SkipJSONPaths   []string `koanf:"skip_json_paths"`   // Global JSON paths to skip
	TestProbability uint64   `koanf:"test_probability"`  // Default: 100
}

// ComputedRouteConfig represents a fully resolved route configuration for runtime use
type ComputedRouteConfig struct {
	CompareHeaders  bool     // Resolved boolean value
	CompareBody     bool     // Resolved boolean value
	SkipHeaders     []string // Headers to skip during comparison
	StoreReqBody    bool     // Resolved boolean value
	StoreRespBodies bool     // Resolved boolean value
	SkipJSONPaths   []string // JSON paths to skip
	TestProbability uint64   // Test probability percentage
}

// ComputedRouteConfigs contains pre-computed route configurations for fast runtime lookup
type ComputedRouteConfigs struct {
	// Pre-computed route configs: "GET:/api/users" -> merged config
	Routes map[string]ComputedRouteConfig

	// Pre-computed global config (with legacy migration applied)
	Global ComputedRouteConfig

	// Skip routes for fast lookup: "GET:/health" -> true
	SkipRoutes map[string]bool
}

// LoadHTTP function will load the file located in path and return the parsed config for ProksiHTTP. This function will panic on errors
func LoadHTTP(path string) *HTTPConfig {
	// Create a fresh koanf instance for each load to avoid state pollution
	localK := koanf.New(".")

	// LoadHTTP default config in the beginning
	err := localK.Load(structs.Provider(defaultHTTP, "koanf"), nil)
	if err != nil {
		logging.L.Fatal("error in loading the default config", zap.Error(err))
	}

	// LoadHTTP YAML config and merge into the previously loaded config.
	err = localK.Load(file.Provider(path), yaml.Parser())
	if err != nil {
		logging.L.Fatal("error in loading the config file", zap.Error(err))
	}

	var c HTTPConfig
	err = localK.Unmarshal("", &c)
	if err != nil {
		logging.L.Fatal("error in unmarshalling the config file", zap.Error(err))
	}

	// Apply backward compatibility migrations
	c.migrateFromLegacyConfig()

	// Validate route patterns
	c.validateRoutePatterns()

	// Pre-compute route configurations for fast runtime lookup
	ComputedConfigs = c.PrecomputeRouteConfigs()

	HTTP = &c
	return &c
}

// migrateFromLegacyConfig migrates legacy configuration fields to new GlobalConfig structure
func (c *HTTPConfig) migrateFromLegacyConfig() {
	// Migration strategy: Only migrate legacy fields if they differ from the default values
	// AND the global config appears to be using defaults (not explicitly configured)

	// Check if global config seems to be explicitly configured by seeing if it differs from defaults
	defaultGlobal := defaultHTTP.GlobalConfig
	globalConfigIsDefault := (c.GlobalConfig.CompareHeaders == defaultGlobal.CompareHeaders &&
		c.GlobalConfig.StoreReqBody == defaultGlobal.StoreReqBody &&
		c.GlobalConfig.StoreRespBodies == defaultGlobal.StoreRespBodies &&
		c.GlobalConfig.TestProbability == defaultGlobal.TestProbability &&
		len(c.GlobalConfig.SkipJSONPaths) == len(defaultGlobal.SkipJSONPaths))

	// Only migrate if global config appears to be using defaults (not explicitly set)
	if !globalConfigIsDefault {
		// Global config was explicitly configured, don't override with legacy values
		return
	}

	// Migrate CompareHeaders
	if c.CompareHeaders != defaultHTTP.CompareHeaders {
		c.GlobalConfig.CompareHeaders = c.CompareHeaders
	}

	// Migrate TestProbability
	if c.TestProbability != 0 && c.TestProbability != defaultHTTP.TestProbability {
		c.GlobalConfig.TestProbability = c.TestProbability
	}

	// Migrate LogResponsePayload to StoreRespBodies
	if c.LogResponsePayload != defaultHTTP.LogResponsePayload {
		c.GlobalConfig.StoreRespBodies = c.LogResponsePayload
	}

	// Migrate SkipJSONPaths
	if len(c.SkipJSONPaths) > 0 && len(c.GlobalConfig.SkipJSONPaths) == 0 {
		c.GlobalConfig.SkipJSONPaths = c.SkipJSONPaths
	}
}

// validateRoutePatterns validates route patterns at startup to catch invalid patterns early
func (c *HTTPConfig) validateRoutePatterns() {
	validatePatterns := func(routes []string, context string) {
		for _, route := range routes {
			_, path := ParseRoute(route)
			if !isValidRoutePattern(path) {
				logging.L.Fatal(fmt.Sprintf("Invalid route pattern in %s: %s", context, route))
			}
		}
	}

	// Validate skip routes
	validatePatterns(c.SkipRoutes, "skip_routes")

	// Validate route configs
	routeConfigKeys := make([]string, 0, len(c.RouteConfigs))
	for route := range c.RouteConfigs {
		routeConfigKeys = append(routeConfigKeys, route)
	}
	validatePatterns(routeConfigKeys, "route_configs")
}

// isValidRoutePattern validates that a route pattern is well-formed
func isValidRoutePattern(path string) bool {
	// Empty path is invalid
	if path == "" {
		return false
	}

	// Path should start with / or be a single *
	if !strings.HasPrefix(path, "/") && path != "*" {
		return false
	}

	// Check for invalid wildcard combinations
	if strings.Contains(path, "**") {
		return false // Double wildcards not supported
	}

	// Check for invalid trailing patterns
	if strings.HasSuffix(path, "*") && !strings.HasSuffix(path, "/*") && path != "*" {
		return false // Only /* or single * allowed at end
	}

	return true
}

// FormatRoute formats HTTP method and path into a route string
func FormatRoute(method, path string) string {
	return fmt.Sprintf("%s:%s", strings.ToUpper(method), path)
}

// ParseRoute parses a route string into method and path components
func ParseRoute(route string) (method, path string) {
	parts := strings.SplitN(route, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	// If no method specified, assume wildcard
	return "*", route
}

// MatchRoute checks if a request route matches a configured route pattern
func MatchRoute(requestRoute, configRoute string) bool {
	requestMethod, requestPath := ParseRoute(requestRoute)
	configMethod, configPath := ParseRoute(configRoute)

	// Check method match (wildcard "*" matches any method)
	if configMethod != "*" && configMethod != requestMethod {
		return false
	}

	// Check path match
	return matchPath(requestPath, configPath)
}

// matchPath checks if a request path matches a configured path pattern
func matchPath(requestPath, configPath string) bool {
	// Exact match
	if requestPath == configPath {
		return true
	}

	// Enhanced wildcard matching for route parameters
	if strings.Contains(configPath, "*") {
		return matchSegmentWildcards(requestPath, configPath)
	}

	// Path pattern match using Go's path.Match
	matched, _ := path.Match(configPath, requestPath)
	return matched
}

// matchSegmentWildcards handles segment-aware wildcard matching
func matchSegmentWildcards(requestPath, configPath string) bool {
	// Handle trailing /* pattern only when it's the ONLY wildcard in the pattern
	// This matches patterns like "/api/v1/*" but forces segment matching for "/api/*/v1/*" or "/*/*"
	if strings.HasSuffix(configPath, "/*") {
		segments := strings.Split(strings.Trim(configPath, "/"), "/")
		// Only treat as trailing wildcard if there are no other wildcards in the pattern
		hasOtherWildcards := false
		for i, seg := range segments {
			if seg == "*" && i != len(segments)-1 {
				hasOtherWildcards = true
				break
			}
		}

		if !hasOtherWildcards {
			// This is a true trailing wildcard with no other wildcards
			prefix := strings.TrimSuffix(configPath, "/*")
			return strings.HasPrefix(requestPath, prefix)
		}
	}

	// Split paths into segments for segment-by-segment comparison
	requestSegments := strings.Split(strings.Trim(requestPath, "/"), "/")
	configSegments := strings.Split(strings.Trim(configPath, "/"), "/")

	// Handle empty path case
	if len(requestSegments) == 1 && requestSegments[0] == "" {
		requestSegments = []string{}
	}
	if len(configSegments) == 1 && configSegments[0] == "" {
		configSegments = []string{}
	}

	// Must have same number of segments for exact segment matching
	if len(requestSegments) != len(configSegments) {
		return false
	}

	// Compare each segment
	for i, configSeg := range configSegments {
		if configSeg == "*" {
			// Single * matches any single segment (route parameter)
			continue
		}
		if configSeg != requestSegments[i] {
			return false
		}
	}

	return true
}

// PrecomputeRouteConfigs creates pre-computed route configurations for fast runtime lookup
func (c *HTTPConfig) PrecomputeRouteConfigs() *ComputedRouteConfigs {
	computed := &ComputedRouteConfigs{
		Routes:     make(map[string]ComputedRouteConfig),
		SkipRoutes: make(map[string]bool),
	}

	// Pre-compute global config (with legacy migration applied)
	computed.Global = ComputedRouteConfig{
		CompareHeaders:  c.GlobalConfig.CompareHeaders,
		CompareBody:     c.GlobalConfig.CompareBody,
		SkipHeaders:     append([]string{}, c.GlobalConfig.SkipHeaders...),
		StoreReqBody:    c.GlobalConfig.StoreReqBody,
		StoreRespBodies: c.GlobalConfig.StoreRespBodies,
		SkipJSONPaths:   append([]string{}, c.GlobalConfig.SkipJSONPaths...),
		TestProbability: c.GlobalConfig.TestProbability,
	}

	logging.L.Info("global config", zap.Any("config", computed.Global))

	// Pre-compute skip routes for fast lookup
	for _, skipRoute := range c.SkipRoutes {
		computed.SkipRoutes[skipRoute] = true
	}

	// Pre-compute route-specific configurations
	for routePattern, routeConfig := range c.RouteConfigs {
		// Start with global config as base
		mergedConfig := ComputedRouteConfig{
			CompareHeaders:  computed.Global.CompareHeaders,
			CompareBody:     computed.Global.CompareBody,
			SkipHeaders:     append([]string{}, computed.Global.SkipHeaders...),
			StoreReqBody:    computed.Global.StoreReqBody,
			StoreRespBodies: computed.Global.StoreRespBodies,
			SkipJSONPaths:   append([]string{}, computed.Global.SkipJSONPaths...),
			TestProbability: computed.Global.TestProbability,
		}

		// Override with route-specific config using semantic keywords
		switch routeConfig.CompareHeaders {
		case "enable":
			mergedConfig.CompareHeaders = true
		case "disable":
			mergedConfig.CompareHeaders = false
		}
		// Empty string means inherit from global (no override needed)

		// Override with route-specific config using semantic keywords
		switch routeConfig.CompareBody {
		case "enable":
			mergedConfig.CompareBody = true
		case "disable":
			mergedConfig.CompareBody = false
		}
		// Empty string means inherit from global (no override needed)

		switch routeConfig.StoreReqBody {
		case "enable":
			mergedConfig.StoreReqBody = true
		case "disable":
			mergedConfig.StoreReqBody = false
		}
		// Empty string means inherit from global (no override needed)

		switch routeConfig.StoreRespBodies {
		case "enable":
			mergedConfig.StoreRespBodies = true
		case "disable":
			mergedConfig.StoreRespBodies = false
		}
		// Empty string means inherit from global (no override needed)

		if len(routeConfig.SkipHeaders) > 0 {
			mergedConfig.SkipHeaders = append(mergedConfig.SkipHeaders, routeConfig.SkipHeaders...)
		}
		if len(routeConfig.SkipJSONPaths) > 0 {
			mergedConfig.SkipJSONPaths = append(mergedConfig.SkipJSONPaths, routeConfig.SkipJSONPaths...)
		}
		if routeConfig.TestProbability > 0 {
			mergedConfig.TestProbability = routeConfig.TestProbability
		}

		// Store the pre-computed config
		computed.Routes[routePattern] = mergedConfig

		logging.L.Info("route_config", zap.String("pattern", routePattern), zap.Any("config", mergedConfig))
	}

	return computed
}

// GetRouteConfig returns pre-computed route configuration for runtime lookup
func GetRouteConfig(route string) ComputedRouteConfig {
	// Check for exact match first (for performance)
	if config, exists := ComputedConfigs.Routes[route]; exists {
		return config
	}

	// Check for pattern matches using MatchRoute
	for configRoute, config := range ComputedConfigs.Routes {
		if MatchRoute(route, configRoute) {
			return config
		}
	}

	// Return global config if no specific route config found
	return ComputedConfigs.Global
}

// IsRouteSkipped checks if a route should be skipped using pre-computed lookup
func IsRouteSkipped(route string) bool {
	// Check for exact match first (for performance)
	if ComputedConfigs.SkipRoutes[route] {
		return true
	}

	// Check for pattern matches using MatchRoute
	for skipRoute := range ComputedConfigs.SkipRoutes {
		if MatchRoute(route, skipRoute) {
			return true
		}
	}

	return false
}
