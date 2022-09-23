package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"

	"github.com/elastic/go-elasticsearch/v8"
	"go.uber.org/zap"

	"github.com/snapp-incubator/proksi/internal/config"
	"github.com/snapp-incubator/proksi/internal/logging"
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
	var err error

	// Usage Demo
	if help {
		flag.Usage()
		return
	}

	c := config.Load(configPath)

	if c.Upstreams.Main.Address == "" {
		logging.L.Fatal("Main upstream backend can not be empty.")
	}

	if c.Upstreams.Test.Address == "" {
		logging.L.Fatal("Test upstream backend can not be empty.")
	}

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

	mux := http.NewServeMux()
	mux.HandleFunc("/", new(server).handle)

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

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)
	<-sigint

	logging.L.Debug("Closing HTTP connections")
	if err := srv.Shutdown(context.Background()); err != nil {
		logging.L.Error("Error in shutting down the HTTP server", zap.Error(err))
	}

	logging.L.Info("HTTP server is shut down")
}

type server struct{}

func (s *server) handle(writer http.ResponseWriter, req *http.Request) {
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
	mainReq, err := http.NewRequestWithContext(req.Context(), req.Method, config.HTTP.Upstreams.Main.Address+req.URL.String(), reqBodyReader)
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

	go func() {
		_, err = reqBodyReader.Seek(0, io.SeekStart)
		if err != nil {
			logging.L.Error("error in seeking the body reader to the first of the stream", loggingFieldsWithError(err)...)
			return
		}

		testReq, err := http.NewRequestWithContext(context.Background(), req.Method, config.HTTP.Upstreams.Test.Address+req.URL.String(), reqBodyReader)
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

		_, err = mainResBodyReader.Seek(0, io.SeekStart)
		if err != nil {
			logging.L.Error("error in seeking to the beginning of the main service response", loggingFieldsWithError(err)...)
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

		if testRes.StatusCode != mainRes.StatusCode {
			logging.L.Warn("Different status code from services", loggingFields(mainRes.StatusCode, testRes.StatusCode)...)
			err = strg.Store(storage.Log{
				URL:                    req.URL.String(),
				Headers:                req.Header,
				MainUpstreamStatusCode: mainRes.StatusCode,
				TestUpstreamStatusCode: testRes.StatusCode,
			})
			if err != nil {
				logging.L.Error("Error in logging the request into Storage", loggingFieldsWithError(err)...)
			}
			return
		}

		equalBody, err := JSONBytesEqual(mainResBody, testResBody)
		if err != nil {
			logging.L.Error("error in JSON equality check of body request", loggingFieldsWithError(err)...)
			return
		}

		if equalBody {
			logging.L.Info("Equal body response", loggingFields(mainRes.StatusCode, testRes.StatusCode)...)
		} else {
			logging.L.Warn("NOT equal body response", loggingFields(mainRes.StatusCode, testRes.StatusCode)...)
			err = strg.Store(storage.Log{
				URL:                    req.URL.String(),
				Headers:                req.Header,
				MainUpstreamStatusCode: mainRes.StatusCode,
				TestUpstreamStatusCode: testRes.StatusCode,
			})
			if err != nil {
				logging.L.Error("Error in logging the request into Storage", loggingFieldsWithError(err)...)
			}
		}
	}()
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
