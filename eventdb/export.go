package eventdb

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// jsonHeader is written as the first line of the events file.
type jsonHeader struct {
	Version int `json:"version"`
}

// jsonEvent is the envelope written for each event line.
type jsonEvent struct {
	Type string    `json:"type"`
	Time time.Time `json:"time"`

	// PhaseEvent fields
	Name string `json:"name,omitempty"`

	// ActionEvent / ErrorEvent fields
	Message string `json:"message,omitempty"`

	// ErrorEvent fields
	Workload string `json:"workload,omitempty"`
	Code     string `json:"code,omitempty"`
	Details  string `json:"details,omitempty"`
	Count    int64  `json:"count,omitempty"`

	// RunInfoEvent fields
	RunID string `json:"run_id,omitempty"`

	// MetricsEvent fields
	Metrics *jsonMetrics `json:"metrics,omitempty"`

	// WorkerMetricsEvent / DinoSystemMetricsEvent fields
	CPUPercent *float64 `json:"cpu_percent,omitempty"`
	RSSBytes   *int64   `json:"rss_bytes,omitempty"`
}

type jsonMetrics struct {
	SuccessCount int64 `json:"success_count"`
	FailureCount int64 `json:"failure_count"`
	AvgLatencyUs int64 `json:"avg_latency_us"`
	MinLatencyUs int64 `json:"min_latency_us"`
	MaxLatencyUs int64 `json:"max_latency_us"`
	P50LatencyUs int64 `json:"p50_latency_us"`
	P90LatencyUs int64 `json:"p90_latency_us"`
	P99LatencyUs int64 `json:"p99_latency_us"`
}

// WriteJSON writes the event log to w as line-delimited JSON.
// The first line is a version header {"version":1}, followed by
// one JSON object per line for each event.
func WriteJSON(w io.Writer, log *EventLog) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	// Write version header.
	if err := enc.Encode(jsonHeader{Version: 1}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	for _, e := range log.Events() {
		var je jsonEvent

		switch ev := e.(type) {
		case StartEvent:
			je = jsonEvent{
				Type: "start",
				Time: ev.Time,
			}
		case EndEvent:
			je = jsonEvent{
				Type: "end",
				Time: ev.Time,
			}
		case RunInfoEvent:
			je = jsonEvent{
				Type:  "run_info",
				Time:  ev.Time,
				RunID: ev.RunID,
			}
		case PhaseEvent:
			je = jsonEvent{
				Type: "phase",
				Time: ev.Time,
				Name: ev.Name,
			}
		case ActionEvent:
			je = jsonEvent{
				Type:    "action",
				Time:    ev.Time,
				Message: ev.Message,
			}
		case MetricsEvent:
			s := ev.Summary
			je = jsonEvent{
				Type:     "metrics",
				Time:     s.Timestamp,
				Workload: ev.Workload,
				Metrics: &jsonMetrics{
					SuccessCount: s.SuccessCount,
					FailureCount: s.FailureCount,
					AvgLatencyUs: s.AvgLatency.Microseconds(),
					MinLatencyUs: s.MinLatency.Microseconds(),
					MaxLatencyUs: s.MaxLatency.Microseconds(),
					P50LatencyUs: s.P50Latency.Microseconds(),
					P90LatencyUs: s.P90Latency.Microseconds(),
					P99LatencyUs: s.P99Latency.Microseconds(),
				},
			}
		case ErrorEvent:
			je = jsonEvent{
				Type:     "error",
				Time:     ev.Time,
				Workload: ev.Workload,
				Code:     ev.Code,
				Details:  ev.Details,
				Count:    ev.Count,
			}
		case WorkerMetricsEvent:
			cpu, rss := ev.CPUPercent, ev.RSSBytes
			je = jsonEvent{
				Type:       "worker_metrics",
				Time:       ev.Time,
				CPUPercent: &cpu,
				RSSBytes:   &rss,
			}
		case DinoSystemMetricsEvent:
			cpu, rss := ev.CPUPercent, ev.RSSBytes
			je = jsonEvent{
				Type:       "dino_system_metrics",
				Time:       ev.Time,
				CPUPercent: &cpu,
				RSSBytes:   &rss,
			}
		default:
			return fmt.Errorf("unknown event type: %T", e)
		}

		if err := enc.Encode(je); err != nil {
			return fmt.Errorf("failed to write event: %w", err)
		}
	}

	return nil
}
