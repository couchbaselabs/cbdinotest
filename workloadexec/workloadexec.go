package workloadexec

import (
	"encoding/json"
	"fmt"

	"github.com/couchbaselabs/cbdinotest/jsexec"
	"github.com/dop251/goja"
)

// moduleShims registers the cbdt:* module shims available to workloads.
var moduleShims = map[string]string{
	"cbdt:http": jsexec.CbdtHttpShim,
}

// WorkloadExec holds a compiled workload program, its goja VM, and the
// extracted lifecycle callables. Created via NewWorkloadExec; use Clone
// to create additional instances sharing the same compiled bytecode.
// After Setup() is called, cloned instances automatically carry the
// setup data.
type WorkloadExec struct {
	filePath string
	program  *goja.Program
	info     WorkloadInfo

	vm         *jsexec.Runtime
	setupFn    goja.Callable
	defaultFn  goja.Callable
	teardownFn goja.Callable

	workerURI string // stored for Clone

	// setupDataJSON is the JSON-serialized form of setup()'s return
	// value. Storing as JSON validates serializability at setup time
	// and ensures each Clone gets an independent deserialized copy.
	setupDataJSON []byte
	// setupDataVal is the goja.Value of the setup data in this VM.
	setupDataVal goja.Value
}

// NewWorkloadExec bundles and compiles the workload script at filePath,
// creates a goja VM, runs the program, and extracts metadata and
// lifecycle callables (setup, default, teardown).
func NewWorkloadExec(filePath string) (*WorkloadExec, error) {
	bundledJS, err := jsexec.Bundle(filePath, "__workload", moduleShims)
	if err != nil {
		return nil, fmt.Errorf("failed to bundle workload: %w", err)
	}

	prog, err := goja.Compile(filePath, bundledJS, false)
	if err != nil {
		return nil, fmt.Errorf("failed to compile workload: %w", err)
	}

	return newWorkloadExecFromProgram(filePath, prog)
}

// newWorkloadExecFromProgram creates a WorkloadExec by running the
// already-compiled program in a fresh goja VM.
func newWorkloadExecFromProgram(filePath string, prog *goja.Program) (*WorkloadExec, error) {
	vm := jsexec.NewRuntime()

	// Set up core globals and modules for workloads.
	jsexec.SetupConsole(vm)
	jsexec.SetupCodedError(vm)
	jsexec.SetupUtils(vm)
	jsexec.SetupCbdtHttp(vm)

	// Execute the compiled program to populate exports.
	if _, err := vm.RunProgram(prog); err != nil {
		return nil, fmt.Errorf("failed to execute workload program: %w", err)
	}

	// Extract metadata from the options export.
	info, err := extractWorkloadInfo(vm)
	if err != nil {
		return nil, fmt.Errorf("failed to extract workload info: %w", err)
	}

	workloadObj := vm.Get("__workload").ToObject(vm.Runtime)

	setupFn, err := jsexec.ExtractFunc(workloadObj, "setup")
	if err != nil {
		return nil, fmt.Errorf("workload missing setup export: %w", err)
	}

	teardownFn, err := jsexec.ExtractFunc(workloadObj, "teardown")
	if err != nil {
		return nil, fmt.Errorf("workload missing teardown export: %w", err)
	}

	defaultFn, err := jsexec.ExtractFunc(workloadObj, "default")
	if err != nil {
		return nil, fmt.Errorf("workload missing default export: %w", err)
	}

	return &WorkloadExec{
		filePath:   filePath,
		program:    prog,
		info:       info,
		vm:         vm,
		setupFn:    setupFn,
		defaultFn:  defaultFn,
		teardownFn: teardownFn,
	}, nil
}

// Clone creates a new WorkloadExec from the same compiled bytecode
// in a fresh goja VM. If setup data has been set (via Setup or
// SetSetupData), it is automatically imported into the new VM.
func (we *WorkloadExec) Clone() (*WorkloadExec, error) {
	clone, err := newWorkloadExecFromProgram(we.filePath, we.program)
	if err != nil {
		return nil, err
	}

	if clone.info != we.info {
		return nil, fmt.Errorf("workload metadata mismatch: expected %+v, got %+v", we.info, clone.info)
	}

	// Carry the worker URI and setup data into the clone.
	if we.workerURI != "" {
		clone.SetWorkerURI(we.workerURI)
	}
	if we.setupDataJSON != nil {
		// Deserialize a fresh copy so each clone has its own
		// independent Go map — goja wraps Go maps directly,
		// so sharing would cause concurrent map access panics.
		clone.setupDataJSON = we.setupDataJSON

		var deserialized interface{}
		if err := json.Unmarshal(we.setupDataJSON, &deserialized); err != nil {
			return nil, fmt.Errorf("failed to deserialize setup data: %w", err)
		}
		clone.setupDataVal = clone.vm.ToValue(deserialized)
	}

	return clone, nil
}

// Info returns the workload metadata extracted during construction.
func (we *WorkloadExec) Info() WorkloadInfo {
	return we.info
}

// SetWorkerURI sets the WORKER_URI global on the goja VM and stores
// the value so Clone can carry it automatically.
func (we *WorkloadExec) SetWorkerURI(uri string) {
	we.workerURI = uri
	we.vm.Set("WORKER_URI", uri)
}

// Setup calls the workload's exported setup() function. The return
// value is serialized to JSON for safe cloning and stored as a
// goja.Value for Execute/Teardown in this VM.
func (we *WorkloadExec) Setup() error {
	result, err := we.setupFn(goja.Undefined())
	if err != nil {
		return fmt.Errorf("workload setup failed: %w", err)
	}

	we.setupDataVal = result
	if result != nil && !goja.IsUndefined(result) && !goja.IsNull(result) {
		exported := result.Export()
		serialized, err := json.Marshal(exported)
		if err != nil {
			return fmt.Errorf("workload setup data is not JSON-serializable: %w", err)
		}

		we.setupDataJSON = serialized
	}

	return nil
}

// Execute calls the workload's exported default() function once,
// passing the internal setup data as the first argument.
func (we *WorkloadExec) Execute() error {
	_, err := we.defaultFn(goja.Undefined(), we.setupDataVal)
	return err
}

// Teardown calls the workload's exported teardown() function,
// passing the internal setup data as the first argument.
func (we *WorkloadExec) Teardown() error {
	if _, err := we.teardownFn(goja.Undefined(), we.setupDataVal); err != nil {
		return fmt.Errorf("workload teardown failed: %w", err)
	}
	return nil
}

// Interrupt interrupts the goja VM. Safe to call from another goroutine.
func (we *WorkloadExec) Interrupt(v interface{}) {
	we.vm.Interrupt(v)
}

// extractWorkloadInfo reads name and description from the __workload.options export.
func extractWorkloadInfo(vm *jsexec.Runtime) (WorkloadInfo, error) {
	workloadVal := vm.Get("__workload")
	if workloadVal == nil || goja.IsUndefined(workloadVal) {
		return WorkloadInfo{}, fmt.Errorf("workload module not found")
	}

	workloadObj := workloadVal.ToObject(vm.Runtime)
	optsVal := workloadObj.Get("options")
	if optsVal == nil || goja.IsUndefined(optsVal) {
		return WorkloadInfo{}, fmt.Errorf("workload does not export 'options'")
	}

	optsObj := optsVal.ToObject(vm.Runtime)
	info := WorkloadInfo{}

	if v := optsObj.Get("name"); v != nil && !goja.IsUndefined(v) {
		info.Name = v.String()
	}

	if v := optsObj.Get("description"); v != nil && !goja.IsUndefined(v) {
		info.Description = v.String()
	}

	return info, nil
}

// jsErrorToMetricsError converts a JS error (from a goja function call)
// into a code and details string. If the error is a CodedError, the code
// and details are extracted; otherwise the raw error string is used.
func jsErrorToMetricsError(we *WorkloadExec, err error) (code string, details string) {
	if exc, ok := err.(*goja.Exception); ok {
		if code, details, ok := jsexec.IsCodedError(we.vm.Runtime, exc.Value()); ok {
			return code, details
		}
	}
	return "unknown", err.Error()
}
