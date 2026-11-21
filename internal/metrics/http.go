package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/snapp-incubator/proksi/internal/logging"
)

var buckets = []float64{
	0.0005,
	0.001, // 1ms
	0.002,
	0.005,
	0.01, // 10ms
	0.02,
	0.05,
	0.1, // 100 ms
	0.2,
	0.5,
	1.0, // 1s
	2.0,
	5.0,
	10.0, // 10s
	15.0,
	20.0,
	30.0,
}

var (
	HTTPReqCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "proksi",
		Subsystem: "http",
		Name:      "request_count",
		Help:      "HTTP Request count",
	}, []string{"status", "method", "upstream"})

	HTTPReqDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "proksi",
		Subsystem: "http",
		Name:      "request_duration",
		Help:      "Duration of each request",
		Buckets:   buckets,
	}, []string{"method", "upstream"})
)

// InitializeHTTP initialize the metrics
func InitializeHTTP(bind string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	srv := http.Server{
		Addr:    bind,
		Handler: mux,
	}
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		logging.L.Fatal("Error in HTTP server ListenAndServe", zap.Error(err))
	}
}
