package metrics

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/couchbaselabs/cbdinotest/eventdb"
)

// LatencyRange captures the min, max, and average of a latency metric
// across multiple second summaries.
type LatencyRange struct {
	Min time.Duration
	Max time.Duration
	Avg time.Duration
}

// MetricsSummary aggregates all per-second metrics from a set of events.
type MetricsSummary struct {
	Seconds      int
	SuccessCount int64
	FailureCount int64

	AvgLatency time.Duration
	MinLatency time.Duration
	MaxLatency time.Duration
	P50Latency LatencyRange
	P90Latency LatencyRange
	P99Latency LatencyRange
}

// SuccessRate returns the ratio of successful operations to total operations.
// Returns 0 if there are no operations. If the computed rate exceeds 99.9%
// but at least one failure occurred, it is capped at 99.9% to avoid
// misleadingly showing 100%.
func (s *MetricsSummary) SuccessRate() float64 {
	total := s.SuccessCount + s.FailureCount
	if total == 0 {
		return 0
	}
	rate := float64(s.SuccessCount) / float64(total)

	if rate > 0.999 && s.FailureCount > 0 {
		// Cap at 99.9% to avoid misleadingly showing 100% when there are failures
		return 0.999
	}

	return rate
}

// WorkerMetricsSummary aggregates worker CPU and memory usage over a phase.
type WorkerMetricsSummary struct {
	Samples int
	MinCPU  float64
	MaxCPU  float64
	AvgCPU  float64
	MinRSS  int64
	MaxRSS  int64
	AvgRSS  int64
}

// ErrorSummaryEntry represents an aggregated error code with its total count.
type ErrorSummaryEntry struct {
	Code    string
	Details string
	Count   int64
}

// PhaseMetrics holds per-workload summaries for a single phase.
type PhaseMetrics struct {
	Phase         string
	Duration      time.Duration
	Workloads     map[string]*MetricsSummary
	Errors        map[string][]ErrorSummaryEntry // keyed by workload name
	WorkerMetrics *WorkerMetricsSummary
	DinoMetrics   *WorkerMetricsSummary
}

// SummarizeMetrics aggregates metrics across all MetricsEvents in the
// given event slice, grouped by phase and workload name. Returns an
// ordered slice of PhaseMetrics (one per phase). Returns an error if
// no phases are found in the event slice.
func SummarizeMetrics(events []eventdb.Event) ([]PhaseMetrics, error) {
	phases := Phases(events)

	if len(phases) == 0 {
		return nil, fmt.Errorf("no phases found in events")
	}

	// Bucket MetricsEvents into phases by timestamp.
	var allPhaseMetrics []PhaseMetrics
	hasAny := false

	for _, phase := range phases {
		phaseEvents := EventsBetween(events, phase.Start, phase.End)
		grouped := make(map[string][]eventdb.SecondSummary)
		for _, e := range phaseEvents {
			if me, ok := e.(eventdb.MetricsEvent); ok {
				grouped[me.Workload] = append(grouped[me.Workload], me.Summary)
			}
		}

		workloads := make(map[string]*MetricsSummary, len(grouped))
		for name, summaries := range grouped {
			workloads[name] = summarizeSecondSummaries(summaries)
			hasAny = true
		}

		allPhaseMetrics = append(allPhaseMetrics, PhaseMetrics{
			Phase:         phase.Name,
			Duration:      phase.End.Sub(phase.Start),
			Workloads:     workloads,
			Errors:        summarizeErrors(phaseEvents),
			WorkerMetrics: summarizeWorkerMetrics(phaseEvents),
			DinoMetrics:   summarizeDinoMetrics(phaseEvents),
		})
	}

	if !hasAny {
		return allPhaseMetrics, nil
	}

	return allPhaseMetrics, nil
}

func summarizeSecondSummaries(summaries []eventdb.SecondSummary) *MetricsSummary {
	var (
		totalSuccess int64
		totalFailure int64

		sumAvgLat int64
		minLat    = time.Duration(math.MaxInt64)
		maxLat    = time.Duration(0)

		sumP50 int64
		minP50 = time.Duration(math.MaxInt64)
		maxP50 = time.Duration(0)

		sumP90 int64
		minP90 = time.Duration(math.MaxInt64)
		maxP90 = time.Duration(0)

		sumP99 int64
		minP99 = time.Duration(math.MaxInt64)
		maxP99 = time.Duration(0)
	)

	for _, s := range summaries {
		totalSuccess += s.SuccessCount
		totalFailure += s.FailureCount

		sumAvgLat += int64(s.AvgLatency)

		if s.MinLatency < minLat {
			minLat = s.MinLatency
		}
		if s.MaxLatency > maxLat {
			maxLat = s.MaxLatency
		}

		sumP50 += int64(s.P50Latency)
		if s.P50Latency < minP50 {
			minP50 = s.P50Latency
		}
		if s.P50Latency > maxP50 {
			maxP50 = s.P50Latency
		}

		sumP90 += int64(s.P90Latency)
		if s.P90Latency < minP90 {
			minP90 = s.P90Latency
		}
		if s.P90Latency > maxP90 {
			maxP90 = s.P90Latency
		}

		sumP99 += int64(s.P99Latency)
		if s.P99Latency < minP99 {
			minP99 = s.P99Latency
		}
		if s.P99Latency > maxP99 {
			maxP99 = s.P99Latency
		}
	}

	n := int64(len(summaries))

	return &MetricsSummary{
		Seconds:      int(n),
		SuccessCount: totalSuccess,
		FailureCount: totalFailure,
		AvgLatency:   time.Duration(sumAvgLat / n),
		MinLatency:   minLat,
		MaxLatency:   maxLat,
		P50Latency: LatencyRange{
			Min: minP50,
			Max: maxP50,
			Avg: time.Duration(sumP50 / n),
		},
		P90Latency: LatencyRange{
			Min: minP90,
			Max: maxP90,
			Avg: time.Duration(sumP90 / n),
		},
		P99Latency: LatencyRange{
			Min: minP99,
			Max: maxP99,
			Avg: time.Duration(sumP99 / n),
		},
	}
}

// summarizeWorkerMetrics aggregates all WorkerMetricsEvents into
// min/max/avg CPU and RSS. Returns nil if no events are found.
func summarizeWorkerMetrics(events []eventdb.Event) *WorkerMetricsSummary {
	var (
		count  int
		sumCPU float64
		minCPU = math.MaxFloat64
		maxCPU = -1.0
		sumRSS int64
		minRSS = int64(math.MaxInt64)
		maxRSS = int64(0)
	)

	for _, e := range events {
		wm, ok := e.(eventdb.WorkerMetricsEvent)
		if !ok {
			continue
		}

		count++
		sumCPU += wm.CPUPercent
		if wm.CPUPercent < minCPU {
			minCPU = wm.CPUPercent
		}
		if wm.CPUPercent > maxCPU {
			maxCPU = wm.CPUPercent
		}

		sumRSS += wm.RSSBytes
		if wm.RSSBytes < minRSS {
			minRSS = wm.RSSBytes
		}
		if wm.RSSBytes > maxRSS {
			maxRSS = wm.RSSBytes
		}
	}

	if count == 0 {
		return nil
	}

	return &WorkerMetricsSummary{
		Samples: count,
		MinCPU:  minCPU,
		MaxCPU:  maxCPU,
		AvgCPU:  sumCPU / float64(count),
		MinRSS:  minRSS,
		MaxRSS:  maxRSS,
		AvgRSS:  sumRSS / int64(count),
	}
}

// summarizeDinoMetrics aggregates all DinoSystemMetricsEvents into
// min/max/avg CPU and RSS. Returns nil if no events are found.
func summarizeDinoMetrics(events []eventdb.Event) *WorkerMetricsSummary {
	var (
		count  int
		sumCPU float64
		minCPU = math.MaxFloat64
		maxCPU = -1.0
		sumRSS int64
		minRSS = int64(math.MaxInt64)
		maxRSS = int64(0)
	)

	for _, e := range events {
		dm, ok := e.(eventdb.DinoSystemMetricsEvent)
		if !ok {
			continue
		}

		count++
		sumCPU += dm.CPUPercent
		if dm.CPUPercent < minCPU {
			minCPU = dm.CPUPercent
		}
		if dm.CPUPercent > maxCPU {
			maxCPU = dm.CPUPercent
		}

		sumRSS += dm.RSSBytes
		if dm.RSSBytes < minRSS {
			minRSS = dm.RSSBytes
		}
		if dm.RSSBytes > maxRSS {
			maxRSS = dm.RSSBytes
		}
	}

	if count == 0 {
		return nil
	}

	return &WorkerMetricsSummary{
		Samples: count,
		MinCPU:  minCPU,
		MaxCPU:  maxCPU,
		AvgCPU:  sumCPU / float64(count),
		MinRSS:  minRSS,
		MaxRSS:  maxRSS,
		AvgRSS:  sumRSS / int64(count),
	}
}

// summarizeErrors aggregates ErrorEvents by workload and error code,
// returning a map of workload name to sorted error entries (highest count first).
func summarizeErrors(events []eventdb.Event) map[string][]ErrorSummaryEntry {
	// workload -> code -> aggregated entry
	type key struct {
		workload string
		code     string
	}

	agg := make(map[key]*ErrorSummaryEntry)

	for _, e := range events {
		ee, ok := e.(eventdb.ErrorEvent)
		if !ok {
			continue
		}

		k := key{workload: ee.Workload, code: ee.Code}
		entry, exists := agg[k]
		if !exists {
			agg[k] = &ErrorSummaryEntry{
				Code:    ee.Code,
				Details: ee.Details,
				Count:   ee.Count,
			}
		} else {
			entry.Count += ee.Count
		}
	}

	if len(agg) == 0 {
		return nil
	}

	// Group by workload.
	result := make(map[string][]ErrorSummaryEntry)
	for k, entry := range agg {
		result[k.workload] = append(result[k.workload], *entry)
	}

	// Sort each workload's errors by count descending.
	for name := range result {
		entries := result[name]
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Count > entries[j].Count
		})
		result[name] = entries
	}

	return result
}

func printErrorSummary(errors []ErrorSummaryEntry, indent string) {
	const maxDisplay = 3
	shown := errors
	if len(shown) > maxDisplay {
		shown = shown[:maxDisplay]
	}

	fmt.Printf("%sErrors:\n", indent)
	for _, e := range shown {
		fmt.Printf("%s  [%s] x%d: %s\n", indent, e.Code, e.Count, e.Details)
	}

	remaining := len(errors) - len(shown)
	if remaining > 0 {
		fmt.Printf("%s  ... and %d more error types\n", indent, remaining)
	}
}

// PrintSummaryOptions controls which sections are included in the summary.
type PrintSummaryOptions struct {
	IncludePhases  bool
	IncludeActions bool
}

// PrintRunSummary prints a formatted summary of the run including
// timeline and per-workload metrics. Start and end times are derived
// from the first and last events in the log (StartEvent / EndEvent).
// Phases and actions are included based on the provided options.
func PrintRunSummary(title string, eventLog *eventdb.EventLog, opts PrintSummaryOptions) {
	events := eventLog.Events()
	if len(events) == 0 {
		fmt.Println("(no events recorded)")
		return
	}

	startTime := events[0].EventTime()
	endTime := events[len(events)-1].EventTime()

	fmt.Println()
	fmt.Println("========================================")
	fmt.Printf("          %s\n", title)
	fmt.Println("========================================")

	// Timeline.
	fmt.Println()
	fmt.Printf("  Started:  %s\n", startTime.Format(time.RFC3339))
	fmt.Printf("  Ended:    %s\n", endTime.Format(time.RFC3339))
	fmt.Printf("  Duration: %s\n", endTime.Sub(startTime).Truncate(time.Millisecond))

	// Workloads involved.
	workloadNames := collectWorkloadNames(events)
	if len(workloadNames) > 0 {
		fmt.Println()
		fmt.Println("  Workloads:")
		for _, name := range workloadNames {
			fmt.Printf("    - %s\n", name)
		}
	}

	if opts.IncludePhases {
		phases := Phases(events)
		if len(phases) > 0 {
			fmt.Println()
			fmt.Println("  Phases:")
			for _, p := range phases {
				elapsed := p.Start.Sub(startTime).Truncate(time.Millisecond)
				dur := p.End.Sub(p.Start).Truncate(time.Millisecond)
				fmt.Printf("    %-20s %s [+%s] (duration: %s)\n",
					p.Name, p.Start.Format("15:04:05.000"), elapsed, dur)
			}
		}
	}

	if opts.IncludeActions {
		var actions []eventdb.ActionEvent
		for _, e := range events {
			if ae, ok := e.(eventdb.ActionEvent); ok {
				actions = append(actions, ae)
			}
		}
		if len(actions) > 0 {
			fmt.Println()
			fmt.Println("  Actions:")
			for _, a := range actions {
				elapsed := a.Time.Sub(startTime).Truncate(time.Millisecond)
				fmt.Printf("    %s [+%s] %s\n", a.Time.Format("15:04:05.000"), elapsed, a.Message)
			}
		}
	}

	// Per-phase, per-workload metrics summaries.
	fmt.Println()
	fmt.Println("  Workload Metrics:")
	phaseSummaries, err := SummarizeMetrics(events)
	if err != nil {
		fmt.Printf("    error: %s\n", err)
	} else if len(phaseSummaries) == 0 {
		fmt.Println("    (no metrics captured)")
	} else {
		for _, pm := range phaseSummaries {
			if len(pm.Workloads) == 0 {
				continue
			}
			fmt.Printf("    [%s] (%s)\n", pm.Phase, pm.Duration.Truncate(time.Millisecond))
			for name, ms := range pm.Workloads {
				fmt.Printf("      %s:\n", name)
				printMetricsSummary(ms, "        ")
				if errs, ok := pm.Errors[name]; ok && len(errs) > 0 {
					printErrorSummary(errs, "        ")
				}
			}
			if pm.WorkerMetrics != nil {
				fmt.Println("      Worker:")
				printWorkerMetricsSummary(pm.WorkerMetrics, "        ")
			}
			if pm.DinoMetrics != nil {
				fmt.Println("      Dinotest:")
				printWorkerMetricsSummary(pm.DinoMetrics, "        ")
			}
		}
	}

	fmt.Println()
}

func printMetricsSummary(ms *MetricsSummary, indent string) {
	var reqPerSec float64
	if ms.Seconds > 0 {
		reqPerSec = float64(ms.SuccessCount) / float64(ms.Seconds)
	}
	fmt.Printf("%sSuccess: %d (%.1f/s)  Failure: %d  Success Rate: %.1f%%\n",
		indent, ms.SuccessCount, reqPerSec, ms.FailureCount, ms.SuccessRate()*100)
	fmt.Printf("%sLatency avg: %s  min: %s  max: %s\n",
		indent,
		ms.AvgLatency.Truncate(time.Microsecond),
		ms.MinLatency.Truncate(time.Microsecond),
		ms.MaxLatency.Truncate(time.Microsecond))
	fmt.Printf("%sP50: %s - %s (avg %s)\n",
		indent,
		ms.P50Latency.Min.Truncate(time.Microsecond),
		ms.P50Latency.Max.Truncate(time.Microsecond),
		ms.P50Latency.Avg.Truncate(time.Microsecond))
	fmt.Printf("%sP90: %s - %s (avg %s)\n",
		indent,
		ms.P90Latency.Min.Truncate(time.Microsecond),
		ms.P90Latency.Max.Truncate(time.Microsecond),
		ms.P90Latency.Avg.Truncate(time.Microsecond))
	fmt.Printf("%sP99: %s - %s (avg %s)\n",
		indent,
		ms.P99Latency.Min.Truncate(time.Microsecond),
		ms.P99Latency.Max.Truncate(time.Microsecond),
		ms.P99Latency.Avg.Truncate(time.Microsecond))
}

func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func printWorkerMetricsSummary(wm *WorkerMetricsSummary, indent string) {
	fmt.Printf("%sCPU: %.1f%% avg  %.1f%% min  %.1f%% max\n",
		indent, wm.AvgCPU, wm.MinCPU, wm.MaxCPU)
	fmt.Printf("%sRSS: %s avg  %s min  %s max\n",
		indent, formatBytes(wm.AvgRSS), formatBytes(wm.MinRSS), formatBytes(wm.MaxRSS))
}

// collectWorkloadNames returns a deduplicated list of workload names
// from MetricsEvents in the order they first appear.
func collectWorkloadNames(events []eventdb.Event) []string {
	seen := make(map[string]bool)
	var names []string
	for _, e := range events {
		if me, ok := e.(eventdb.MetricsEvent); ok {
			if !seen[me.Workload] {
				seen[me.Workload] = true
				names = append(names, me.Workload)
			}
		}
	}
	return names
}
