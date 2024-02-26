package storage

// Log defines the structure of records storing in Storage as log of requests
type Log struct {
	URL                         string              `json:"url"`
	Headers                     map[string][]string `json:"headers"`
	MainUpstreamStatusCode      int                 `json:"main_upstream_status_code"`
	TestUpstreamStatusCode      int                 `json:"test_upstream_status_code"`
	MainUpstreamResponsePayload *string             `json:"main_upstream_response_payload"`
	TestUpstreamResponsePayload *string             `json:"test_upstream_response_payload"`
}
