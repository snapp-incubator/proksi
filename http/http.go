package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"reflect"
	"strings"

	"go.uber.org/zap"

	"github.com/anvari1313/proksi/internal/logging"
)

var (
	mainServiceClient = &http.Client{}
	testServiceClient = &http.Client{}
)

var (
	help        bool   // Indicates whether to show the help or not
	configPath  string // Path of config file
	bindAddress string // Address of the HTTP server to bind

	mainUpstream string // Main upstream backend
	testUpstream string // Test upstream backend
)

func init() {
	flag.BoolVar(&help, "help", false, "Show help")
	flag.StringVar(&configPath, "config", "", "The path of config file")
	flag.StringVar(&bindAddress, "bind", ":3333", "Address of the HTTP server to be bind to")
	flag.StringVar(&mainUpstream, "main-upstream", "", "Address of the main service upstream backend")
	flag.StringVar(&testUpstream, "test-upstream", "", "Address of the test service upstream backend")

	// Parse the terminal flags
	flag.Parse()
}

func main() {
	// Usage Demo
	if help {
		flag.Usage()
		return
	}

	if mainUpstream == "" {
		logging.L.Fatal("Main upstream backend can not be empty.")
	}

	if testUpstream == "" {
		logging.L.Fatal("Test upstream backend can not be empty.")
	}

	http.HandleFunc("/", handler)

	err := http.ListenAndServe(bindAddress, http.DefaultServeMux)
	if err != nil {
		panic(err)
	}
}

func handler(writer http.ResponseWriter, req *http.Request) {
	loggingFieldsWithError := func(err error) []zap.Field {
		return []zap.Field{
			zap.String("method", req.Method),
			zap.String("url", req.URL.String()),
			zap.Error(err),
		}
	}

	loggingFields := func(mainStatusCode, testStatusCode int) []zap.Field {
		return []zap.Field{
			zap.String("method", req.Method),
			zap.String("url", req.URL.String()),
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
	mainReq, err := http.NewRequestWithContext(req.Context(), req.Method, mainUpstream+req.URL.String(), reqBodyReader)
	if err != nil {
		logging.L.Error("error in creating the request to the main service", loggingFieldsWithError(err)...)
		return
	}

	mainReq.Header = req.Header
	mainRes, err := mainServiceClient.Do(mainReq)
	if err != nil {
		logging.L.Error("error in doing the request to the main service", loggingFieldsWithError(err)...)
		return
	}

	_, err = reqBodyReader.Seek(0, io.SeekStart)
	if err != nil {
		logging.L.Error("error in seeking the body reader to the first of the stream", loggingFieldsWithError(err)...)
		return
	}

	testReq, err := http.NewRequestWithContext(req.Context(), req.Method, testUpstream+req.URL.String(), reqBodyReader)
	if err != nil {
		logging.L.Error("error in creating the request to the test service", loggingFieldsWithError(err)...)
		return
	}

	testReq.Header = req.Header
	testRes, err := testServiceClient.Do(testReq)
	if err != nil {
		logging.L.Error("error in doing the request to the test service", loggingFieldsWithError(err)...)
		return
	}

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
		logging.L.Error("error in copying the main service response into the byte buffer", loggingFieldsWithError(err)...)
		return
	}

	mainResBodyReader := bytes.NewReader(mainResBodyBuffer.Bytes())
	_, err = io.Copy(writer, mainResBodyReader)
	if err != nil {
		logging.L.Error("error in writing the response to the response writer", loggingFieldsWithError(err)...)
		return
	}

	_, err = mainResBodyReader.Seek(0, io.SeekStart)
	if err != nil {
		logging.L.Error("error in seeking to the beginning of the main service response", loggingFieldsWithError(err)...)
		return
	}

	if testRes.StatusCode != mainRes.StatusCode {
		logging.L.Warn("Different status code from services", loggingFields(mainRes.StatusCode, testRes.StatusCode)...)
		return
	}

	mainResBody, err := io.ReadAll(mainResBodyReader)
	if err != nil {
		logging.L.Error("error in reading the body request of main service", loggingFieldsWithError(err)...)
		return
	}
	defer func() { _ = mainRes.Body.Close() }()

	testResBody, err := io.ReadAll(testRes.Body)
	if err != nil {
		logging.L.Error("error in reading the body request of test service", loggingFieldsWithError(err)...)
		return
	}
	defer func() { _ = testRes.Body.Close() }()

	equalBody, err := JSONBytesEqual(mainResBody, testResBody)
	if err != nil {
		logging.L.Error("error in JSON equality check of body request", loggingFieldsWithError(err)...)
		return
	}

	if equalBody {
		logging.L.Info("Equal body response", loggingFields(mainRes.StatusCode, testRes.StatusCode)...)
	} else {
		logging.L.Warn("NOT equal body response", loggingFields(mainRes.StatusCode, testRes.StatusCode)...)
	}
}

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
