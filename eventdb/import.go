package eventdb

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// ReadJSON reads a line-delimited JSON event log (as produced by
// WriteJSON) from r and returns a populated EventLog.
func ReadJSON(r io.Reader) (*EventLog, error) {
	scanner := bufio.NewScanner(r)

	// Read version header.
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read header: %w", err)
		}
		return nil, fmt.Errorf("empty event log file")
	}

	var header jsonHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}
	if header.Version != 1 {
		return nil, fmt.Errorf("unsupported event log version: %d", header.Version)
	}

	el := NewEmptyEventLog()

	for scanner.Scan() {
		var je jsonEvent
		if err := json.Unmarshal(scanner.Bytes(), &je); err != nil {
			return nil, fmt.Errorf("failed to parse event: %w", err)
		}

		event, err := decodeEvent(je)
		if err != nil {
			return nil, err
		}

		el.Add(event)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read event log: %w", err)
	}

	return el, nil
}

func decodeEvent(je jsonEvent) (Event, error) {
	switch je.Type {
	case "start":
		return StartEvent{Time: je.Time}, nil
	case "end":
		return EndEvent{Time: je.Time}, nil
	case "run_info":
		return RunInfoEvent{Time: je.Time, RunID: je.RunID}, nil
	case "phase":
		return PhaseEvent{Time: je.Time, Name: je.Name}, nil
	case "action":
		return ActionEvent{Time: je.Time, Message: je.Message}, nil
	case "metrics":
		if je.Metrics == nil {
			return nil, fmt.Errorf("metrics event missing metrics data")
		}
		m := je.Metrics
		return MetricsEvent{
			Workload: je.Workload,
			Summary: SecondSummary{
				Timestamp:    je.Time,
				SuccessCount: m.SuccessCount,
				FailureCount: m.FailureCount,
				AvgLatency:   time.Duration(m.AvgLatencyUs) * time.Microsecond,
				MinLatency:   time.Duration(m.MinLatencyUs) * time.Microsecond,
				MaxLatency:   time.Duration(m.MaxLatencyUs) * time.Microsecond,
				P50Latency:   time.Duration(m.P50LatencyUs) * time.Microsecond,
				P90Latency:   time.Duration(m.P90LatencyUs) * time.Microsecond,
				P99Latency:   time.Duration(m.P99LatencyUs) * time.Microsecond,
			},
		}, nil
	case "error":
		return ErrorEvent{
			Time:     je.Time,
			Workload: je.Workload,
			Code:     je.Code,
			Details:  je.Details,
			Count:    je.Count,
		}, nil
	case "worker_metrics":
		var cpu float64
		var rss int64
		if je.CPUPercent != nil {
			cpu = *je.CPUPercent
		}
		if je.RSSBytes != nil {
			rss = *je.RSSBytes
		}
		return WorkerMetricsEvent{Time: je.Time, CPUPercent: cpu, RSSBytes: rss}, nil
	case "dino_system_metrics":
		var cpu float64
		var rss int64
		if je.CPUPercent != nil {
			cpu = *je.CPUPercent
		}
		if je.RSSBytes != nil {
			rss = *je.RSSBytes
		}
		return DinoSystemMetricsEvent{Time: je.Time, CPUPercent: cpu, RSSBytes: rss}, nil
	default:
		return nil, fmt.Errorf("unknown event type: %q", je.Type)
	}
}
