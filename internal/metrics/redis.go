package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/snapp-incubator/proksi/internal/logging"
)

var (
	// RedisCmdCounter is the total number of redis commands proxied, partitioned by
	// the command name and the upstream (main_upstream/test_upstream).
	RedisCmdCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "proksi",
		Subsystem: "redis",
		Name:      "command_count",
		Help:      "Redis command count",
	}, []string{"command", "upstream"})

	// RedisCmdDuration is the duration of each redis command proxied to an upstream.
	RedisCmdDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "proksi",
		Subsystem: "redis",
		Name:      "command_duration",
		Help:      "Duration of each redis command",
		Buckets:   buckets,
	}, []string{"command", "upstream"})
)

// InitializeRedis initialize the metrics server for Proksi Redis.
func InitializeRedis(bind string) {
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