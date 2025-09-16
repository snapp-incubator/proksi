package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/snapp-incubator/proksi/internal/logging"
)


// 1ms to 10s
var buckets = []float64{0.001, 0.002, 0.005, 0.01, 0.1, 1.0, 10.0}

var (
	HTTPReqCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "proksi",
		Subsystem: "http",
		Name:      "request_count",
		Help:      "HTTP Request count",
	}, []string{"status", "upstream"})

	HTTPReqDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "proksi",
		Subsystem: "http",
		Name:      "request_duration",
		Help:      "Duration of each request",
		Buckets:   buckets,
	}, []string{"upstream"})

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

	HeaderComparisonCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "proksi",
		Subsystem: "http",
		Name:      "header_comparison_results",
		Help:      "Results of header comparisons",
	}, []string{"result"})

	StatusCode2xxVsNon2xxCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "proksi",
		Subsystem: "http",
		Name:      "status_2xx_vs_non2xx_count",
		Help:      "Counter for cases where main upstream returns 2xx but test upstream returns non-2xx",
	})
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
