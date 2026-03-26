package jsexec

import (
	"fmt"
	"time"

	"github.com/couchbaselabs/cbdinotest/eventdb"
	"github.com/dop251/goja"
)

// EventLogger is the interface for recording phase transitions and events.
type EventLogger interface {
	Add(e eventdb.Event)
}

// SetupMetrics registers the global Metrics object (markPhase) on the runtime.
// Phase transitions are recorded via the given EventLogger.
func SetupMetrics(vm *Runtime, logger EventLogger) {
	metricsObj := vm.NewObject()
	metricsObj.Set("markPhase", func(call goja.FunctionCall) goja.Value {
		phase := call.Argument(0).String()
		fmt.Printf("[METRICS] marking phase: %s\n", phase)
		logger.Add(eventdb.PhaseEvent{Time: time.Now(), Name: phase})
		return goja.Undefined()
	})
	vm.Set("Metrics", metricsObj)
}
