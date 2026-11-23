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

	ComparisonResults = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "proksi",
		Subsystem: "http",
		Name:      "comparison_results",
		Help:      "Results of upstream response comparisons",
	}, []string{"diff_type"})

	RouteSkipCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "proksi",
		Subsystem: "http",
		Name:      "route_skips",
		Help:      "Counter for skipped routes",
	}, []string{"skip_reason"})

	StatusCode2xxVsNon2xxCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "proksi",
		Subsystem: "http",
		Name:      "status_2xx_vs_non2xx_count",
		Help:      "Counter for cases where main upstream returns 2xx but test upstream returns non-2xx",
	})
)

// HTTPMismatchCounter counts responses that differ between main and test
// upstreams. The type label is one of "status_code", "content_type" or "body".
var HTTPMismatchCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "proksi",
	Subsystem: "http",
	Name:      "mismatch_count",
	Help:      "Number of main/test response mismatches by type",
}, []string{"type"})

// RegisterQueueDepth exposes the number of pending test jobs as a gauge.
func RegisterQueueDepth(depth func() float64) {
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "proksi",
		Subsystem: "http",
		Name:      "queue_depth",
		Help:      "Number of mirror jobs waiting in the worker queue",
	}, depth)
}

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
