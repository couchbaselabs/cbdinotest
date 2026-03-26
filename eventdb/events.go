package eventdb

import (
	"sort"
	"sync"
	"time"
)

// --- Event types ---

// Event is one of PhaseEvent, ActionEvent, or MetricsEvent.
type Event interface {
	EventTime() time.Time
	// EventEndTime returns the effective end time for this event.
	// For instantaneous events this equals EventTime. For events
	// that span a duration (e.g. MetricsEvent) it returns the end
	// of the covered period.
	EventEndTime() time.Time
}

// PhaseEvent marks a phase transition (e.g. "setup", "warmup", "run").
type PhaseEvent struct {
	Time time.Time
	Name string
}

func (e PhaseEvent) EventTime() time.Time    { return e.Time }
func (e PhaseEvent) EventEndTime() time.Time { return e.Time }

// ActionEvent records when the situation performed a notable action.
type ActionEvent struct {
	Time    time.Time
	Message string
}

func (e ActionEvent) EventTime() time.Time    { return e.Time }
func (e ActionEvent) EventEndTime() time.Time { return e.Time }

// ErrorEvent captures an error that occurred during a particular second.
// At most one ErrorEvent is recorded per second per unique error code.
type ErrorEvent struct {
	Time     time.Time
	Workload string
	Code     string
	Details  string
	Count    int64
}

func (e ErrorEvent) EventTime() time.Time    { return e.Time }
func (e ErrorEvent) EventEndTime() time.Time { return e.Time }

// SecondSummary is the per-second aggregate of all operations.
type SecondSummary struct {
	Timestamp    time.Time
	SuccessCount int64
	FailureCount int64

	AvgLatency time.Duration
	MinLatency time.Duration
	MaxLatency time.Duration
	P50Latency time.Duration
	P90Latency time.Duration
	P99Latency time.Duration
}

// MetricsEvent wraps a SecondSummary for a 1-second metrics slice.
type MetricsEvent struct {
	Workload string
	Summary  SecondSummary
}

func (e MetricsEvent) EventTime() time.Time    { return e.Summary.Timestamp }
func (e MetricsEvent) EventEndTime() time.Time { return e.Summary.Timestamp.Add(time.Second) }

// StartEvent marks the start of the event log.
type StartEvent struct {
	Time time.Time
}

func (e StartEvent) EventTime() time.Time    { return e.Time }
func (e StartEvent) EventEndTime() time.Time { return e.Time }

// EndEvent marks the end of the event log.
type EndEvent struct {
	Time time.Time
}

func (e EndEvent) EventTime() time.Time    { return e.Time }
func (e EndEvent) EventEndTime() time.Time { return e.Time }

// RunInfoEvent records metadata about the run, emitted immediately
// after the StartEvent.
type RunInfoEvent struct {
	Time  time.Time
	RunID string
}

func (e RunInfoEvent) EventTime() time.Time    { return e.Time }
func (e RunInfoEvent) EventEndTime() time.Time { return e.Time }

// WorkerMetricsEvent captures periodic CPU and memory usage from the worker process.
type WorkerMetricsEvent struct {
	Time       time.Time
	CPUPercent float64
	RSSBytes   int64
}

func (e WorkerMetricsEvent) EventTime() time.Time    { return e.Time }
func (e WorkerMetricsEvent) EventEndTime() time.Time { return e.Time }

// DinoSystemMetricsEvent captures periodic CPU and memory usage from the
// cbdinotest process itself.
type DinoSystemMetricsEvent struct {
	Time       time.Time
	CPUPercent float64
	RSSBytes   int64
}

func (e DinoSystemMetricsEvent) EventTime() time.Time    { return e.Time }
func (e DinoSystemMetricsEvent) EventEndTime() time.Time { return e.Time }

// --- EventLog ---

// EventLog is a thread-safe, append-only log of events.
type EventLog struct {
	mu     sync.Mutex
	events []Event
}

// NewEventLog creates a new EventLog, seeded with a StartEvent.
func NewEventLog() *EventLog {
	el := &EventLog{}
	el.Add(StartEvent{Time: time.Now()})
	return el
}

// NewEmptyEventLog creates a new EventLog without any seed events.
// This is useful when importing events from an external source.
func NewEmptyEventLog() *EventLog {
	return &EventLog{}
}

// Add inserts an event into the log in time-sorted order.
// Because metrics events may arrive after later wall-clock events,
// we insert by scanning backwards from the end. This is O(1) when
// events arrive in order, and handles the occasional late arrival.
func (l *EventLog) Add(e Event) {
	l.mu.Lock()
	defer l.mu.Unlock()

	t := e.EventTime()

	// Fast path: event is at or after the last entry (most common).
	if len(l.events) == 0 || !t.Before(l.events[len(l.events)-1].EventTime()) {
		l.events = append(l.events, e)
		return
	}

	// Scan backwards to find the insertion point.
	i := len(l.events) - 1
	for i > 0 && t.Before(l.events[i-1].EventTime()) {
		i--
	}

	// Insert at position i.
	l.events = append(l.events, nil)
	copy(l.events[i+1:], l.events[i:])
	l.events[i] = e
}


// Finalize appends an EndEvent with the current time, marking the
// definitive end of the event log. After appending the end event,
// the log is sorted by event time to account for metrics events
// that may have been emitted out of order due to async finalization.
func (l *EventLog) Finalize() {
	l.Add(EndEvent{Time: time.Now()})

	l.mu.Lock()
	defer l.mu.Unlock()
	sort.SliceStable(l.events, func(i, j int) bool {
		return l.events[i].EventTime().Before(l.events[j].EventTime())
	})
}

// Events returns a copy of all events recorded so far.
func (l *EventLog) Events() []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Event, len(l.events))
	copy(out, l.events)
	return out
}
