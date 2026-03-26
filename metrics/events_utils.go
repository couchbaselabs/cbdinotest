package metrics

import (
	"time"

	"github.com/couchbaselabs/cbdinotest/eventdb"
)

// Phase describes a named phase and the time period it covers.
type Phase struct {
	Name  string
	Start time.Time
	End   time.Time
}

// Phases extracts all phases from a sorted event slice. An implicit
// "_default" entry is prepended to cover the period from the first
// event to the first named phase. Each subsequent phase ends when the
// next one begins. The final phase ends at the timestamp of the last
// event in the slice.
func Phases(events []eventdb.Event) []Phase {
	if len(events) == 0 {
		return nil
	}

	var namedPhases []Phase
	for _, e := range events {
		if pe, ok := e.(eventdb.PhaseEvent); ok {
			namedPhases = append(namedPhases, Phase{
				Name:  pe.Name,
				Start: pe.Time,
			})
		}
	}

	// Build the full phase list starting with _default.
	firstTime := events[0].EventTime()
	lastTime := events[len(events)-1].EventTime()

	var phases []Phase

	if len(namedPhases) > 0 {
		// _default runs from the first event to the first named phase.
		phases = append(phases, Phase{
			Name:  "_default",
			Start: firstTime,
			End:   namedPhases[0].Start,
		})

		phases = append(phases, namedPhases...)

		// Each named phase ends when the next one starts.
		for i := 1; i < len(phases)-1; i++ {
			phases[i].End = phases[i+1].Start
		}

		// The last phase ends at the time of the final event.
		phases[len(phases)-1].End = lastTime
	} else {
		// No named phases; the entire run is _default.
		phases = append(phases, Phase{
			Name:  "_default",
			Start: firstTime,
			End:   lastTime,
		})
	}

	return phases
}

// EventsBetween returns all events whose end time falls in (start, end].
// This uses EventEndTime so that events spanning a duration (such as
// MetricsEvents covering a 1-second bucket) are attributed to the phase
// that came later, which is typically what we want since most of the time
// phase changes come with immediate changes to the system state.
// The input slice must be sorted by EventTime.
func EventsBetween(events []eventdb.Event, start, end time.Time) []eventdb.Event {
	var result []eventdb.Event
	for _, e := range events {
		t := e.EventEndTime()
		if !t.After(start) {
			continue
		}
		if t.After(end) {
			continue
		}
		result = append(result, e)
	}
	return result
}
