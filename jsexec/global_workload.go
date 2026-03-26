package jsexec

import (
	"fmt"

	"github.com/dop251/goja"
)

// WorkloadRunner is the interface that a workload runner must implement
// to be usable from the Workload global.
type WorkloadRunner interface {
	Setup(setupOpts interface{}) error
	Start() error
	Stop() error
}

// SetupWorkload registers the global Workload object (setup/start/stop) on the runtime.
// The runner is invoked when JS code calls Workload.setup/start/stop.
func SetupWorkload(vm *Runtime, runner WorkloadRunner) {
	workload := vm.NewObject()

	workload.Set("setup", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0)
		if arg == nil || goja.IsUndefined(arg) {
			panic(vm.NewGoError(fmt.Errorf("Workload.setup requires an options argument")))
		}

		opts := arg.Export()
		fmt.Printf("[WORKLOAD] setting up workload with opts=%v\n", opts)

		if err := runner.Setup(opts); err != nil {
			panic(vm.NewGoError(fmt.Errorf("workload setup failed: %w", err)))
		}

		return goja.Undefined()
	})

	workload.Set("start", func(call goja.FunctionCall) goja.Value {
		fmt.Println("[WORKLOAD] starting workload")

		if err := runner.Start(); err != nil {
			panic(vm.NewGoError(fmt.Errorf("workload start failed: %w", err)))
		}

		return goja.Undefined()
	})

	workload.Set("stop", func(call goja.FunctionCall) goja.Value {
		fmt.Println("[WORKLOAD] stopping workload")

		if err := runner.Stop(); err != nil {
			panic(vm.NewGoError(fmt.Errorf("workload stop failed: %w", err)))
		}

		return goja.Undefined()
	})

	vm.Set("Workload", workload)
}
