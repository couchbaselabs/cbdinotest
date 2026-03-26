package scoring

import (
	"fmt"
	"sort"

	"github.com/couchbaselabs/cbdinotest/metrics"
)

// PhaseScoring defines the scoring criteria for a single phase.
type PhaseScoring struct {
	MinSuccessRate  *float64
	MaxSuccessRate  *float64
	MinOpsPerSecond *float64
}

// WorkloadScoring holds per-phase scoring criteria for a single workload.
type WorkloadScoring struct {
	Phases map[string]PhaseScoring // phase name → criteria
}

// Scorer evaluates workload metrics against scoring criteria.
type Scorer struct {
	workloads map[string]WorkloadScoring
}

// NewScorer creates a Scorer from the given per-workload scoring configuration.
func NewScorer(workloads map[string]WorkloadScoring) *Scorer {
	return &Scorer{workloads: workloads}
}

func passFailLabel(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}

// Score evaluates all scoring criteria against the provided phase metrics.
// It prints a [PASS] or [FAIL] line for each individual check and returns
// true only if every check passed.
func (s *Scorer) Score(phases []metrics.PhaseMetrics) bool {
	// Build lookups keyed by phase name.
	phaseLookup := make(map[string]map[string]*metrics.MetricsSummary)
	errorLookup := make(map[string]map[string][]metrics.ErrorSummaryEntry)
	for _, pm := range phases {
		phaseLookup[pm.Phase] = pm.Workloads
		errorLookup[pm.Phase] = pm.Errors
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("              SCORING")
	fmt.Println("========================================")

	allPassed := true

	// Build phase order from the metrics slice (which is chronological),
	// placing _default first if present.
	var phaseOrder []string
	for _, pm := range phases {
		if pm.Phase == "_default" {
			phaseOrder = append([]string{"_default"}, phaseOrder...)
		} else {
			phaseOrder = append(phaseOrder, pm.Phase)
		}
	}

	// Sorted workload names for deterministic output.
	workloadNames := make([]string, 0, len(s.workloads))
	for name := range s.workloads {
		workloadNames = append(workloadNames, name)
	}
	sort.Strings(workloadNames)

	for _, phaseName := range phaseOrder {
		fmt.Println()
		fmt.Printf("  [%s]\n", phaseName)

		// Built-in _default check: no errors allowed.
		if phaseName == "_default" {
			for _, wlName := range workloadNames {
				errs := errorLookup["_default"][wlName]
				var totalCount int64
				for _, e := range errs {
					totalCount += e.Count
				}
				passed := totalCount == 0
				fmt.Printf("    [%s] %s: num_errors <= 0 (actual: %d)\n",
					passFailLabel(passed), wlName, totalCount)
				if !passed {
					allPassed = false
				}
			}
			continue
		}

		// Per-workload scoring checks for this phase.
		for _, wlName := range workloadNames {
			ws := s.workloads[wlName]
			ps, hasCriteria := ws.Phases[phaseName]
			if !hasCriteria {
				continue
			}

			// Look up the MetricsSummary for this workload+phase.
			workloads, phaseFound := phaseLookup[phaseName]
			if !phaseFound {
				if ps.MinSuccessRate != nil || ps.MaxSuccessRate != nil || ps.MinOpsPerSecond != nil {
					fmt.Printf("    [FAIL] %s: phase not found in metrics\n", wlName)
					allPassed = false
				}
				continue
			}

			ms, wlFound := workloads[wlName]
			if !wlFound {
				if ps.MinSuccessRate != nil || ps.MaxSuccessRate != nil || ps.MinOpsPerSecond != nil {
					fmt.Printf("    [FAIL] %s: workload not found in phase metrics\n", wlName)
					allPassed = false
				}
				continue
			}

			actual := ms.SuccessRate()

			if ps.MinSuccessRate != nil {
				threshold := *ps.MinSuccessRate
				passed := actual >= threshold
				fmt.Printf("    [%s] %s: success_rate >= %.1f%% (actual: %.1f%%)\n",
					passFailLabel(passed), wlName, threshold*100, actual*100)
				if !passed {
					allPassed = false
				}
			}

			if ps.MaxSuccessRate != nil {
				threshold := *ps.MaxSuccessRate
				passed := actual <= threshold
				fmt.Printf("    [%s] %s: success_rate <= %.1f%% (actual: %.1f%%)\n",
					passFailLabel(passed), wlName, threshold*100, actual*100)
				if !passed {
					allPassed = false
				}
			}

			if ps.MinOpsPerSecond != nil {
				threshold := *ps.MinOpsPerSecond
				var opsPerSec float64
				if ms.Seconds > 0 {
					opsPerSec = float64(ms.SuccessCount+ms.FailureCount) / float64(ms.Seconds)
				}
				passed := opsPerSec >= threshold
				fmt.Printf("    [%s] %s: ops_per_second >= %.1f (actual: %.1f)\n",
					passFailLabel(passed), wlName, threshold, opsPerSec)
				if !passed {
					allPassed = false
				}
			}
		}
	}

	fmt.Println()

	return allPassed
}
