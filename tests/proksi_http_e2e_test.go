package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	"github.com/snapp-incubator/proksi/internal/logging"
)

// server is HTTP server creator.
func server(address string, initRoutes func(r *mux.Router)) *http.Server {
	r := mux.NewRouter()

	initRoutes(r)

	srv := &http.Server{
		Addr:         address,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	return srv
}

func initMainRoutes(r *mux.Router) {
	r.HandleFunc(
		"/api/test",
		mainServerHandler(),
	).Methods("POST")
}

func initTestRoutes(r *mux.Router) {
	r.HandleFunc(
		"/api/test",
		testServerHandler(),
	).Methods("POST")
}

type requestBody struct {
	Var1 string `json:"var1"`
	Var2 int    `json:"var2"`
	Var3 struct {
		Var4 int    `json:"var4"`
		Var5 string `json:"var5"`
	} `json:"var3"`
}

type responseBody struct {
	Var1 string `json:"var1"`
	Var3 struct {
		Var4 string `json:"var4"`
		Var5 string `json:"var5"`
	} `json:"var3"`
}

func mainServerHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")

		var rq requestBody
		err := json.NewDecoder(r.Body).Decode(&rq)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			logging.L.Warn("error in decode request body", zap.Error(err))
			return
		}

		var res responseBody
		res.Var1 = "success"
		res.Var3.Var4 = "response from main tests"
		res.Var3.Var5 = "processed request"

		err = json.NewEncoder(w).Encode(&res)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			logging.L.Warn("error in encode response body", zap.Error(err))
			return
		}
	}
}

func testServerHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")

		var rq requestBody
		err := json.NewDecoder(r.Body).Decode(&rq)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			logging.L.Warn("error in decode request body", zap.Error(err))
			return
		}

		var res responseBody
		res.Var1 = "success"
		res.Var3.Var4 = "response from test tests"
		res.Var3.Var5 = "processed request"

		err = json.NewEncoder(w).Encode(&res)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			logging.L.Warn("error in encode response body", zap.Error(err))
			return
		}
	}
}

func sendRequest() ([]byte, int, error) {
	url := "127.0.0.1:9090/api/test"
	method := "GET"

	payload := strings.NewReader(`{
    "var1" : "test1",
    "var2" : "test2",
    "var3" : {
        "var4" : "test4",
        "var5" : "test5"
    }
}`)

	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		return nil, 0, err
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, 0, err
	}

	return body, res.StatusCode, nil
}

// ------------------------------------------------------------------
//                     		setup test
// ------------------------------------------------------------------

type HTTPE2ETestSuite struct {
	suite.Suite
	c chan struct{}
}

func (suite *HTTPE2ETestSuite) SetupSuite() {
	suite.c = make(chan struct{}, 1)

	s1 := server("localhost:8080", initMainRoutes)
	logging.L.Info("main tests is running")
	s2 := server("localhost:8081", initTestRoutes)
	logging.L.Info("test tests is running")

	<-suite.c
	s1.Shutdown(context.Background())
	s2.Shutdown(context.Background())

	logging.L.Debug("all servers are down")
}

// ------------------------------------------------------------------
//                     end to end testing
// ------------------------------------------------------------------

func (suite *HTTPE2ETestSuite) TestHTTPE2ETestSuiteSuccess() {
	require := suite.Require()
	// run http Proksi

	// send request to Proksi
	for i := 0; i < 30; i++ {
		_, statusCode, err := sendRequest()
		require.NoError(err)
		require.Equal(http.StatusOK, statusCode)
	}

	require.Equal(1, 1)
	suite.c <- struct{}{}
}

var expectedResponse = `
{"level":"info","ts":1668692988.1142933,"msg":"main tests is running"}
{"level":"info","ts":1668692988.1143281,"msg":"test tests is running"}
{"level":"info","ts":1668692991.6491222,"msg":"Connected to Elasticsearch","info":"[200 OK] {\n  \"name\" : \"1ba7161843fd\",\n  \"cluster_name\" : \"docker-cluster\",\n  \"cluster_uuid\" : \"9p_zFk2OQ7K0GuSPPxPOSw\",\n  \"version\" : {\n    \"number\" : \"8.3.3\",\n    \"build_flavor\" : \"default\",\n    \"build_type\" : \"docker\",\n    \"build_hash\" : \"801fed82df74dbe537f89b71b098ccaff88d2c56\",\n    \"build_date\" : \"2022-07-23T19:30:09.227964828Z\",\n    \"build_snapshot\" : false,\n    \"lucene_version\" : \"9.2.0\",\n    \"minimum_wire_compatibility_version\" : \"7.17.0\",\n    \"minimum_index_compatibility_version\" : \"7.0.0\"\n  },\n  \"tagline\" : \"You Know, for Search\"\n}\n"}
{"level":"info","ts":1668692991.6492991,"msg":"Starting HTTP tests","address":"0.0.0.0:9090","main_upstream":"http://localhost:8080","test_upstream":"http://localhost:8081"}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.8062308,"msg":"Equal body response","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.811579,"msg":"Equal body response","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.8177214,"msg":"Equal body response","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.8238156,"msg":"Equal body response","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.833429,"msg":"Equal body response","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.840634,"msg":"Equal body response","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.8480334,"msg":"Equal body response","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.8556237,"msg":"Equal body response","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"level":"info","ts":1668692991.8624115,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.8691392,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.8770957,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.883696,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.8918407,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9004042,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.906956,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.914256,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.922075,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9276588,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9348607,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9415963,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9478142,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9548082,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.96145,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9671843,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9731517,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9799411,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9868402,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9928732,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692991.9994555,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
{"level":"info","ts":1668692992.0055006,"msg":"Sending request without test upstream","method":"GET","url":"/api/test","main_service_status_code":200,"test_service_status_code":200}
{"var1":"success","var3":{"var4":"response from main tests","var5":"processed request"}}
2022/11/17 17:19:52 http: Server closed
exit status 1
kill: usage: kill [-s sigspec | -n signum | -sigspec] pid | jobspec ... or kill -l [sigspec]
`

// ------------------------------------------------------------------
//                     run test suite
// ------------------------------------------------------------------

func TestHTTPE2ETestSuite(t *testing.T) {
	suite.Run(t, new(HTTPE2ETestSuite))
}
