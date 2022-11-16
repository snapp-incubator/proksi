package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"go.uber.org/zap"

	"github.com/gorilla/mux"

	"github.com/snapp-incubator/proksi/internal/logging"
)

func main() {
	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	s1 := server("localhost:8080", initMainRoutes)
	logging.L.Info("main server is running")
	s2 := server("localhost:8081", initTestRoutes)
	logging.L.Info("test server is running")

	<-c
	shutdown(s1)
	shutdown(s2)
	logging.L.Debug("all servers are down")
}

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

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	return srv
}

func shutdown(srv *http.Server) {
	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(2)*time.Second)
	defer cancel()

	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	err := srv.Shutdown(ctx)
	if err != nil {
		logging.L.Debug("error in shutting down", zap.Error(err))
	}

	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	logging.L.Debug("shutting down")
}

func initMainRoutes(r *mux.Router) {
	r.HandleFunc(
		"/api/test",
		mainServerHandler(),
	).Methods("GET")
}

func initTestRoutes(r *mux.Router) {
	r.HandleFunc(
		"/api/test",
		testServerHandler(),
	).Methods("GET")
}

// handlers
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
		res.Var3.Var4 = "response from main server"
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
		res.Var3.Var4 = "response from test server"
		res.Var3.Var5 = "processed request"

		err = json.NewEncoder(w).Encode(&res)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			logging.L.Warn("error in encode response body", zap.Error(err))
			return
		}
	}
}
