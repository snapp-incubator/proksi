package storage

type Log struct {
	URL                    string              `json:"url"`
	Headers                map[string][]string `json:"headers"`
	MainUpstreamStatusCode int                 `json:"main_upstream_status_code"`
	TestUpstreamStatusCode int                 `json:"test_upstream_status_code"`
}
