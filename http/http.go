package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
)

var (
	mainServiceBaseURL = "http://localhost:3000"
	testServiceBaseURL = "http://localhost:8080"

	mainServiceClient = &http.Client{}
	testServiceClient = &http.Client{}
)

var (
	help       bool   // Indicates whether to show the help or not
	configPath string // Path of config file
)

func init() {
	flag.BoolVar(&help, "help", false, "Show help")
	flag.StringVar(&configPath, "config", "", "path of config file")

	// Parse the terminal flags
	flag.Parse()
}

func main() {
	// Usage Demo
	if help {
		flag.Usage()
		return
	}

	http.HandleFunc("/", handler)

	err := http.ListenAndServe(":3333", http.DefaultServeMux)
	if err != nil {
		panic(err)
	}
}

func handler(writer http.ResponseWriter, req *http.Request) {
	var bodyBuffer bytes.Buffer
	_, err := io.Copy(&bodyBuffer, req.Body)
	if err != nil {
		panic(err)
	}

	bodyReader := bytes.NewReader(bodyBuffer.Bytes())

	mainReq, err := http.NewRequestWithContext(req.Context(), req.Method, mainServiceBaseURL+req.URL.String(), bodyReader)
	if err != nil {
		panic(err)
	}

	mainReq.Header = req.Header
	mainRes, err := mainServiceClient.Do(mainReq)
	if err != nil {
		panic(err)
	}

	_, err = bodyReader.Seek(0, io.SeekStart)
	if err != nil {
		panic(err)
	}

	testReq, err := http.NewRequestWithContext(req.Context(), req.Method, testServiceBaseURL+req.URL.String(), bodyReader)
	if err != nil {
		panic(err)
	}

	testReq.Header = req.Header
	testRes, err := testServiceClient.Do(testReq)
	if err != nil {
		panic(err)
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
		panic(err)
	}

	mainResBodyReader := bytes.NewReader(mainResBodyBuffer.Bytes())
	_, err = io.Copy(writer, mainResBodyReader)
	if err != nil {
		panic(err)
	}

	_, err = mainResBodyReader.Seek(0, io.SeekStart)
	if err != nil {
		panic(err)
	}

	if testRes.StatusCode != mainRes.StatusCode {
		fmt.Printf("[%s] %s - MainService: %d - TestService: %d\tDifferent status codes\n", req.Method, req.URL, mainRes.StatusCode, testRes.StatusCode)
		return
	}

	mainResBody, err := io.ReadAll(mainResBodyReader)
	if err != nil {
		panic(err)
	}
	defer func() { _ = mainRes.Body.Close() }()

	testResBody, err := io.ReadAll(testRes.Body)
	if err != nil {
		panic(err)
	}

	defer func() { _ = testRes.Body.Close() }()

	equalBody, err := JSONBytesEqual(mainResBody, testResBody)
	if err != nil {
		panic(err)
	}

	if equalBody {
		fmt.Printf("[%s] %s - MainService: %d - TestService: %d\tEqual body response\n", req.Method, req.URL, mainRes.StatusCode, testRes.StatusCode)
	} else {
		fmt.Printf("[%s] %s - MainService: %d - TestService: %d\tNOT Equal body response\n", req.Method, req.URL, mainRes.StatusCode, testRes.StatusCode)
		fmt.Println(string(mainResBody))
		fmt.Println(string(testResBody))
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
