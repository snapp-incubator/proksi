package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"

	"github.com/snapp-incubator/proksi/internal/config"
	"github.com/snapp-incubator/proksi/internal/logging"
	"github.com/snapp-incubator/proksi/internal/metrics"
	"github.com/snapp-incubator/proksi/internal/storage"
)

var (
	mainServiceClient = &http.Client{}
	testServiceClient = &http.Client{}

	strg storage.Storage
)

var (
	help       bool   // Indicates whether to show the help or not
	configPath string // Path of config file
)

func init() {
	flag.BoolVar(&help, "help", false, "Show help")
	flag.StringVar(&configPath, "config", "", "The path of config file")

	// Parse the terminal flags
	flag.Parse()
}

func main() {
	// Usage Demo
	if help {
		flag.Usage()
		return
	}

	c := config.LoadHTTP(configPath)

	// Initialize logging with configured level
	if err := logging.InitializeLogger(c.LogLevel); err != nil {
		logging.L.Fatal("Failed to initialize logger", zap.Error(err))
	}

	logging.L.Info("Logger initialized", zap.String("log_level", c.LogLevel))

	if c.Upstreams.Main.Address == "" {
		logging.L.Fatal("Main upstream backend can not be empty.")
	}

	if c.Upstreams.Test.Address == "" {
		logging.L.Fatal("Test upstream backend can not be empty.")
	}

	if config.ComputedConfigs != nil {
		fmt.Printf("computed configs: %+v\n", *config.ComputedConfigs)
	}

	// Initialize storage backend based on configuration
	switch c.StorageType {
	case "stdout":
		strg = &storage.StdoutStorage{}
		logging.L.Info("Using stdout storage backend")
	case "elasticsearch":
		elasticConfig := elasticsearch.Config{
			Addresses:              c.Elasticsearch.Addresses,
			Username:               c.Elasticsearch.Username,
			Password:               c.Elasticsearch.Password,
			CloudID:                c.Elasticsearch.CloudID,
			APIKey:                 c.Elasticsearch.APIKey,
			ServiceToken:           c.Elasticsearch.ServiceToken,
			CertificateFingerprint: c.Elasticsearch.CertificateFingerprint,
		}
		es, err := elasticsearch.NewClient(elasticConfig)
		if err != nil {
			logging.L.Fatal("Error in connecting to Elasticsearch", zap.Error(err))
		}

		esInfo, err := es.Info()
		if err != nil {
			logging.L.Fatal("Error in getting info from Elasticsearch", zap.Error(err))
		}

		logging.L.Info("Connected to Elasticsearch", zap.String("info", esInfo.String()))
		strg = &storage.ElasticStorage{ES: es}
	default:
		logging.L.Fatal("Unknown storage type", zap.String("storage_type", c.StorageType))
	}

	jobs := make(chan Job, c.Worker.QueueSize)

	for i := uint(0); i < c.Worker.Count; i++ {
		go func() {
			for job := range jobs {
				job.Do()
			}
		}()
	}

	mux := http.NewServeMux()
	s := &server{job: jobs}
	mux.HandleFunc("/", s.handle)

	srv := &http.Server{
		Addr:    c.Bind,
		Handler: mux,
	}

	go func() {
		logging.L.Info("Starting HTTP server",
			zap.String("address", c.Bind),
			zap.String("main_upstream", c.Upstreams.Main.Address),
			zap.String("test_upstream", c.Upstreams.Test.Address),
		)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server ListenAndServe Error: %v", err)
		}
	}()

	if c.Metrics.Enabled {
		go metrics.InitializeHTTP(c.Metrics.Bind)
	}

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)
	<-sigint

	logging.L.Debug("Closing HTTP connections")
	if err := srv.Shutdown(context.Background()); err != nil {
		logging.L.Error("Error in shutting down the HTTP server", zap.Error(err))
	}

	logging.L.Info("HTTP server is shut down")
}

type server struct {
	job        chan Job
	reqCounter uint64
}

func (s *server) handle(writer http.ResponseWriter, req *http.Request) {
	route := config.FormatRoute(req.Method, req.URL.Path)

	// Check if route should be skipped entirely
	if config.IsRouteSkipped(route) {
		metrics.RouteSkipCounter.WithLabelValues("config").Inc()

		// For skipped routes, just proxy to main upstream without testing
		reqBodyBuffer := &bytes.Buffer{}
		if _, err := io.Copy(reqBodyBuffer, req.Body); err != nil {
			http.Error(writer, "Failed to read request body", http.StatusBadRequest)
			return
		}

		mainReq, err := http.NewRequestWithContext(req.Context(), req.Method, config.HTTP.Upstreams.Main.Address+req.URL.String(), bytes.NewReader(reqBodyBuffer.Bytes()))
		if err != nil {
			http.Error(writer, "Failed to create upstream request", http.StatusInternalServerError)
			return
		}

		mainReq.Header = req.Header
		t := prometheus.NewTimer(metrics.HTTPReqDuration.WithLabelValues("main_upstream"))
		mainRes, err := mainServiceClient.Do(mainReq)
		t.ObserveDuration()

		if err != nil {
			metrics.HTTPReqCounter.WithLabelValues("client_error", "main_upstream").Inc()
			http.Error(writer, "Failed to reach upstream", http.StatusBadGateway)
			return
		}

		// Copy response
		for headerKey, headerValue := range mainRes.Header {
			if len(headerValue) == 1 {
				writer.Header().Set(headerKey, headerValue[0])
			} else {
				writer.Header().Set(headerKey, "["+strings.Join(headerValue, ",")+"]")
			}
		}
		writer.WriteHeader(mainRes.StatusCode)
		if _, err := io.Copy(writer, mainRes.Body); err != nil {
			logging.L.Error("Failed to copy response body", zap.Error(err))
		}

		metrics.HTTPReqCounter.WithLabelValues(strconv.Itoa(mainRes.StatusCode), "main_upstream").Inc()
		return
	}

	loggingFieldsWithError := func(err error) []zap.Field {
		return []zap.Field{
			zap.String("method", req.Method),
			zap.String("url", req.URL.String()),
			zap.String("route", route),
			zap.Error(err),
		}
	}

	loggingFields := func(mainStatusCode, testStatusCode int) []zap.Field {
		return []zap.Field{
			zap.String("method", req.Method),
			zap.String("url", req.URL.String()),
			zap.String("route", route),
			zap.Int("main_service_status_code", mainStatusCode),
			zap.Int("test_service_status_code", testStatusCode),
		}
	}

	var reqBodyBuffer bytes.Buffer
	_, err := io.Copy(&reqBodyBuffer, req.Body)
	if err != nil {
		logging.L.Error("error in reading the request body", loggingFieldsWithError(err)...)
		return
	}

	reqBodyReader := bytes.NewReader(reqBodyBuffer.Bytes())
	mainReq, err := http.NewRequestWithContext(req.Context(), req.Method, config.HTTP.Upstreams.Main.Address+req.URL.String(), reqBodyReader)
	if err != nil {
		logging.L.Error("error in creating the request to the main service", loggingFieldsWithError(err)...)
		return
	}

	mainReq.Header = req.Header
	t := prometheus.NewTimer(metrics.HTTPReqDuration.WithLabelValues("main_upstream"))
	mainRes, err := mainServiceClient.Do(mainReq)
	t.ObserveDuration()
	if err != nil {
		metrics.HTTPReqCounter.WithLabelValues("client_error", "main_upstream").Inc()
		logging.L.Error("error in doing the request to the main service", loggingFieldsWithError(err)...)
		return
	}

	metrics.HTTPReqCounter.WithLabelValues(strconv.Itoa(mainRes.StatusCode), "main_upstream").Inc()
	// TODO: Array in HTTP header values (issue #1)
	for headerKey, headerValue := range mainRes.Header {
		if len(headerValue) == 1 {
			writer.Header().Set(headerKey, headerValue[0])
		} else {
			writer.Header().Set(headerKey, "["+strings.Join(headerValue, ",")+"]")
		}
	}

	writer.WriteHeader(mainRes.StatusCode)

	var mainResBodyBuffer bytes.Buffer
	_, err = io.Copy(&mainResBodyBuffer, mainRes.Body)
	if err != nil {
		logging.L.Error("error in copying the main upstream response into the byte buffer", loggingFieldsWithError(err)...)
		return
	}

	mainResBodyReader := bytes.NewReader(mainResBodyBuffer.Bytes())
	_, err = io.Copy(writer, mainResBodyReader)
	if err != nil {
		logging.L.Error("error in writing the response to the response writer", loggingFieldsWithError(err)...)
		return
	}

	// Get route-specific configuration
	routeConfig := config.GetRouteConfig(route)

	atomic.AddUint64(&s.reqCounter, 1)
	inBucket := s.reqCounter%100 < routeConfig.TestProbability-1
	if inBucket {
		s.job <- &upstreamTestJob{
			req:                    req,
			route:                  route,
			routeConfig:            routeConfig,
			reqBodyReader:          reqBodyReader,
			reqBodyBuffer:          &reqBodyBuffer,
			loggingFieldsWithError: loggingFieldsWithError,
			loggingFields:          loggingFields,
			mainRes:                mainRes,
			mainResBodyReader:      mainResBodyReader,
		}
	} else {
		logging.L.Info("Sending request without test upstream", loggingFields(mainRes.StatusCode, mainRes.StatusCode)...)
		metrics.HTTPReqCounter.WithLabelValues(strconv.Itoa(mainRes.StatusCode), "test_upstream").Inc()
	}
}

type Job interface {
	Do()
}

type upstreamTestJob struct {
	req           *http.Request
	route         string
	routeConfig   config.ComputedRouteConfig
	reqBodyReader *bytes.Reader
	reqBodyBuffer *bytes.Buffer

	loggingFieldsWithError func(err error) []zap.Field
	loggingFields          func(mainStatusCode, testStatusCode int) []zap.Field

	mainRes           *http.Response
	mainResBodyReader *bytes.Reader
}

func (j *upstreamTestJob) Do() {
	_, err := j.reqBodyReader.Seek(0, io.SeekStart)
	if err != nil {
		logging.L.Error("error in seeking the body reader to the first of the stream", j.loggingFieldsWithError(err)...)
		return
	}

	testReq, err := http.NewRequestWithContext(context.Background(), j.req.Method, config.HTTP.Upstreams.Test.Address+j.req.URL.String(), j.reqBodyReader)
	if err != nil {
		logging.L.Error("error in creating the request to the test service", j.loggingFieldsWithError(err)...)
		return
	}

	testReq.Header = j.req.Header
	t := prometheus.NewTimer(metrics.HTTPReqDuration.WithLabelValues("test_upstream"))
	testRes, err := testServiceClient.Do(testReq)
	t.ObserveDuration()
	if err != nil {
		metrics.HTTPReqCounter.WithLabelValues("client_error", "test_upstream").Inc()
		logging.L.Error("error in doing the request to the test service", j.loggingFieldsWithError(err)...)
		return
	}

	metrics.HTTPReqCounter.WithLabelValues(strconv.Itoa(testRes.StatusCode), "test_upstream").Inc()

	_, err = j.mainResBodyReader.Seek(0, io.SeekStart)
	if err != nil {
		logging.L.Error("error in seeking to the beginning of the main service response", j.loggingFieldsWithError(err)...)
		return
	}

	mainResBody, err := io.ReadAll(j.mainResBodyReader)
	if err != nil {
		logging.L.Error("error in reading the body request of main service", j.loggingFieldsWithError(err)...)
		return
	}
	defer func() { _ = j.mainRes.Body.Close() }()

	testResBody, err := io.ReadAll(testRes.Body)
	if err != nil {
		logging.L.Error("error in reading the body request of test service", j.loggingFieldsWithError(err)...)
		return
	}
	defer func() { _ = testRes.Body.Close() }()

	if testRes.StatusCode != j.mainRes.StatusCode {
		logging.L.Warn("Different status code from services", j.loggingFields(j.mainRes.StatusCode, testRes.StatusCode)...)
		metrics.ComparisonResults.WithLabelValues("status_diff").Inc()

		// Track when main upstream returns 2xx but test upstream returns non-2xx
		if isStatus2xx(j.mainRes.StatusCode) && !isStatus2xx(testRes.StatusCode) {
			metrics.StatusCode2xxVsNon2xxCounter.Inc()
		}

		log := storage.Log{
			URL:                    j.req.URL.String(),
			Method:                 j.req.Method,
			Route:                  j.route,
			Headers:                j.req.Header,
			MainUpstreamStatusCode: j.mainRes.StatusCode,
			TestUpstreamStatusCode: testRes.StatusCode,
			ComparisonType:         "status_diff",
		}

		if j.routeConfig.StoreReqBody {
			reqBody := j.reqBodyBuffer.String()
			log.RequestBody = &reqBody
		}

		err = strg.Store(log)
		if err != nil {
			logging.L.Error("Error in logging the request into Storage", j.loggingFieldsWithError(err)...)
		}
		return
	}

	mainResContentType := j.mainRes.Header.Get("content-type")
	if j.routeConfig.CompareHeaders {
		differentHeaders := j.compareHeaders(j.mainRes.Header, testRes.Header)
		if len(differentHeaders) > 0 {
			logging.L.Warn("Different response headers from services", j.loggingFields(j.mainRes.StatusCode, testRes.StatusCode)...)
			metrics.ComparisonResults.WithLabelValues("header_diff").Inc()

			log := storage.Log{
				URL:                    j.req.URL.String(),
				Method:                 j.req.Method,
				Route:                  j.route,
				Headers:                j.req.Header,
				MainUpstreamStatusCode: j.mainRes.StatusCode,
				TestUpstreamStatusCode: testRes.StatusCode,
				ComparisonType:         "header_diff",
				DifferentHeaders:       differentHeaders,
			}

			if j.routeConfig.StoreReqBody {
				reqBody := j.reqBodyBuffer.String()
				log.RequestBody = &reqBody
			}

			if j.routeConfig.StoreRespBodies {
				mainResBody, _ := io.ReadAll(j.mainResBodyReader)
				testResBody, _ := io.ReadAll(testRes.Body)
				mainResBodyStr := string(mainResBody)
				testResBodyStr := string(testResBody)
				log.MainUpstreamResponsePayload = &mainResBodyStr
				log.TestUpstreamResponsePayload = &testResBodyStr
			}

			err = strg.Store(log)
			if err != nil {
				logging.L.Error("Error in logging the request into Storage", j.loggingFieldsWithError(err)...)
			}
			return
		}
	}

	var comparator bodyEqualizerFunc
	var responseSkipPath bool

	switch strings.ToLower(mainResContentType) {
	case "application/json", "application/ld+json":
		responseSkipPath = true
		comparator = JSONBytesEqual
	// TODO: We didn't have time to implement it.
	// case "application/xml", "application/xhtml+xml", "text/xml":
	//	responseSkipPath = false
	//	comparator = xmlBytesEqual
	default:
		responseSkipPath = false
		comparator = dummyBytesEqual
	}

	equalBody, err := comparator(mainResBody, testResBody)
	if err != nil {
		logging.L.Error("error in response equality check", j.loggingFieldsWithError(err)...)
		return
	}

	if !equalBody && responseSkipPath {
		if testRes.StatusCode == j.mainRes.StatusCode {
			srcBodyStr := string(mainResBody)
			testBodyStr := string(testResBody)

			for i := 0; i < len(j.routeConfig.SkipJSONPaths); i++ {
				srcBodyStr, err = sjson.Set(srcBodyStr, j.routeConfig.SkipJSONPaths[i], "useless")
				if err != nil {
					panic(err)
				}

				testBodyStr, err = sjson.Set(testBodyStr, j.routeConfig.SkipJSONPaths[i], "useless")
				if err != nil {
					panic(err)
				}
			}

			mainResBody = []byte(srcBodyStr)
			testResBody = []byte(testBodyStr)

			equalBody, err = JSONBytesEqual(mainResBody, testResBody)
			if err != nil {
				logging.L.Error("error in JSON equality check of body request", j.loggingFieldsWithError(err)...)
				return
			}
		}
	}

	if equalBody {
		logging.L.Info("Equal body response", j.loggingFields(j.mainRes.StatusCode, testRes.StatusCode)...)
		metrics.ComparisonResults.WithLabelValues("identical").Inc()
	} else {
		logging.L.Warn("NOT equal body response", j.loggingFields(j.mainRes.StatusCode, testRes.StatusCode)...)
		metrics.ComparisonResults.WithLabelValues("body_diff").Inc()

		l := storage.Log{
			URL:                    j.req.URL.String(),
			Method:                 j.req.Method,
			Route:                  j.route,
			Headers:                j.req.Header,
			MainUpstreamStatusCode: j.mainRes.StatusCode,
			TestUpstreamStatusCode: testRes.StatusCode,
			ComparisonType:         "body_diff",
		}

		if j.routeConfig.StoreReqBody {
			reqBody := j.reqBodyBuffer.String()
			l.RequestBody = &reqBody
		}

		if j.routeConfig.StoreRespBodies {
			mainResBodyStr := string(mainResBody)
			testResBodyStr := string(testResBody)
			l.MainUpstreamResponsePayload = &mainResBodyStr
			l.TestUpstreamResponsePayload = &testResBodyStr
		}

		err = strg.Store(l)
		if err != nil {
			logging.L.Error("Error in logging the request into Storage", j.loggingFieldsWithError(err)...)
		}
	}
}

type bodyEqualizerFunc func(a, b []byte) (bool, error)

// JSONBytesEqual compares the JSON in two byte slices.
func JSONBytesEqual(a, b []byte) (bool, error) {
	var json1, json2 interface{}
	if err := json.Unmarshal(a, &json1); err != nil {
		return false, err
	}

	if err := json.Unmarshal(b, &json2); err != nil {
		return false, err
	}

	return reflect.DeepEqual(json2, json1), nil
}


func dummyBytesEqual(a, b []byte) (bool, error) {
	return bytes.Equal(a, b), nil
}

// compareHeaders compares two sets of HTTP headers and returns a list of headers that differ
func (j *upstreamTestJob) compareHeaders(mainHeaders, testHeaders http.Header) []string {
	var differentHeaders []string
	skipHeadersMap := make(map[string]bool)

	// Build skip headers map
	for _, header := range j.routeConfig.SkipHeaders {
		skipHeadersMap[strings.ToLower(header)] = true
	}

	// Check all headers in main response
	for key, mainValues := range mainHeaders {
		keyLower := strings.ToLower(key)
		if skipHeadersMap[keyLower] {
			continue
		}

		testValues, exists := testHeaders[key]
		if !exists {
			differentHeaders = append(differentHeaders, key)
			continue
		}

		// Compare header values
		if len(mainValues) != len(testValues) {
			differentHeaders = append(differentHeaders, key)
			continue
		}

		// Compare each value
		different := false
		for i, mainValue := range mainValues {
			if mainValue != testValues[i] {
				different = true
				break
			}
		}

		if different {
			differentHeaders = append(differentHeaders, key)
		}
	}

	// Check for headers that exist in test but not in main
	for key := range testHeaders {
		keyLower := strings.ToLower(key)
		if skipHeadersMap[keyLower] {
			continue
		}

		if _, exists := mainHeaders[key]; !exists {
			differentHeaders = append(differentHeaders, key)
		}
	}

	return differentHeaders
}

// isStatus2xx returns true if the status code is in the 2xx range (200-299)
func isStatus2xx(statusCode int) bool {
	return statusCode >= 200 && statusCode <= 299
}
