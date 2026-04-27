package controller

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	summaryMaxAge            = time.Second * 30
	summaryAgeBuckets uint32 = 3
	metricNamespace          = "opensandbox-controller"
)

var (
	allocatorScheduleDurationSummary = func() *prometheus.SummaryVec {
		return prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace:  metricNamespace,
				Subsystem:  "allocator",
				Name:       "schedule_duration_ms",
				MaxAge:     summaryMaxAge,
				AgeBuckets: summaryAgeBuckets,
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.95: 0.005, 0.99: 0.001},
			},
			[]string{"namespace", "pool_name", "success"},
		)
	}()
	allocatorPersistAllocationStateDurationSummary = func() *prometheus.SummaryVec {
		return prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace:  metricNamespace,
				Subsystem:  "allocator",
				Name:       "persist_alloc_state_duration_ms",
				MaxAge:     summaryMaxAge,
				AgeBuckets: summaryAgeBuckets,
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.95: 0.005, 0.99: 0.001},
			},
			[]string{"namespace", "pool_name", "success"},
		)
	}()
	allocatorSyncAllocResultDurationSummary = func() *prometheus.SummaryVec {
		return prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace:  metricNamespace,
				Subsystem:  "allocator",
				Name:       "sync_alloc_result_duration_ms",
				MaxAge:     summaryMaxAge,
				AgeBuckets: summaryAgeBuckets,
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.95: 0.005, 0.99: 0.001},
			},
			[]string{"namespace", "pool_name", "success"},
		)
	}()
	allocatorSyncSingleAllocResultDurationSummary = func() *prometheus.SummaryVec {
		return prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace:  metricNamespace,
				Subsystem:  "allocator",
				Name:       "sync_single_alloc_result_duration_ms",
				MaxAge:     summaryMaxAge,
				AgeBuckets: summaryAgeBuckets,
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.95: 0.005, 0.99: 0.001},
			},
			[]string{"namespace", "pool_name", "sandbox_name", "success"},
		)
	}()
)

func init() {
	metrics.Registry.MustRegister(allocatorScheduleDurationSummary)
	metrics.Registry.MustRegister(allocatorPersistAllocationStateDurationSummary)
	metrics.Registry.MustRegister(allocatorSyncAllocResultDurationSummary)
	metrics.Registry.MustRegister(allocatorSyncSingleAllocResultDurationSummary)
}
