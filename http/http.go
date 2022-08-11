package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func main() {
	mainServiceBaseURL := "http://localhost:8080"
	testServiceBaseURL := "http://localhost:3000"

	mainServiceClient := &http.Client{}
	testServiceClient := &http.Client{}

	http.HandleFunc("/", func(writer http.ResponseWriter, req *http.Request) {
		var bodyBuffer bytes.Buffer
		_, err := io.Copy(&bodyBuffer, req.Body)
		if err != nil {
			panic(err)
		}

		bodyReader := bytes.NewReader(bodyBuffer.Bytes())

		mainReq, err := http.NewRequestWithContext(context.Background(), req.Method, mainServiceBaseURL+req.URL.String(), bodyReader)
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

		testReq, err := http.NewRequestWithContext(context.Background(), req.Method, testServiceBaseURL+req.URL.String(), bodyReader)
		if err != nil {
			panic(err)
		}

		testReq.Header = req.Header
		testRes, err := testServiceClient.Do(testReq)
		if err != nil {
			panic(err)
		}

		fmt.Println(testRes)

		for headerKey, headerValue := range mainRes.Header {
			if len(headerValue) == 1 {
				fmt.Print(headerKey, ":", headerValue, "|")
				writer.Header().Set(headerKey, headerValue[0])
			} else {
				fmt.Print(headerKey, ":(", headerValue, ")|")
				writer.Header().Set(headerKey, "["+strings.Join(headerValue, ",")+"]")
			}

		}

		writer.WriteHeader(mainRes.StatusCode)
		fmt.Println()

		_, err = io.Copy(writer, mainRes.Body)
		if err != nil {
			panic(err)
		}

		fmt.Printf("[%s] %s\n", req.Method, req.URL)
	})

	err := http.ListenAndServe(":3333", nil)
	if err != nil {
		panic(err)
	}
}
