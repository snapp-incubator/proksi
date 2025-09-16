package storage

// Log defines the structure of records storing in Storage as log of requests
type Log struct {
	URL                         string              `json:"url"`
	Method                      string              `json:"method"`                 // HTTP method
	Route                       string              `json:"route"`                  // Formatted route (METHOD:/path)
	Headers                     map[string][]string `json:"headers"`                // Request headers
	RequestBody                 *string             `json:"request_body,omitempty"` // Request body (if StoreReqBody is enabled)
	MainUpstreamStatusCode      int                 `json:"main_upstream_status_code"`
	TestUpstreamStatusCode      int                 `json:"test_upstream_status_code"`
	MainUpstreamResponsePayload *string             `json:"main_upstream_response_payload"`
	TestUpstreamResponsePayload *string             `json:"test_upstream_response_payload"`
	ComparisonType              string              `json:"comparison_type,omitempty"`   // "status_diff", "header_diff", "body_diff"
	DifferentHeaders            []string            `json:"different_headers,omitempty"` // List of headers that differed
}
