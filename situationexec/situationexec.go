package situationexec

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/couchbaselabs/cbdinotest/jsexec"
	"github.com/dop251/goja"
)

// moduleShims registers all cbdt:* module shims for esbuild bundling.
var moduleShims = map[string]string{
	"cbdt:http":   jsexec.CbdtHttpShim,
	"cbdt:cbdino": jsexec.CbdtCbdinoShim,
}

// SetupFn is called by SituationExec.Run to let the caller register
// modules and globals on the VM before the program is executed.
type SetupFn func(vm *jsexec.Runtime)

// SituationExec holds pre-compiled situation bytecode.
// Created via NewSituationExec, then executed via Run.
type SituationExec struct {
	filePath string
	program  *goja.Program
	info     *SituationInfo

	// vm is the runtime created during Run, exposed so the caller
	// can interrupt it (e.g. on signal).
	vm *jsexec.Runtime
}

// NewSituationExec bundles and compiles the situation script at filePath,
// then extracts metadata (name, description, parameters, workloads)
// by running the program in a temporary VM. The compiled bytecode
// is retained for re-execution via Run.
func NewSituationExec(filePath string) (*SituationExec, error) {
	bundledJS, err := jsexec.Bundle(filePath, "__workload", moduleShims)
	if err != nil {
		return nil, fmt.Errorf("failed to bundle situation: %w", err)
	}

	prog, err := goja.Compile(filePath, bundledJS, false)
	if err != nil {
		return nil, fmt.Errorf("failed to compile situation: %w", err)
	}

	// Run in a temporary VM to extract metadata.
	vm := jsexec.NewRuntime()
	jsexec.SetupConsole(vm)
	jsexec.SetupCodedError(vm)

	// Register placeholder globals for all cbdt modules so the bundled
	// JS can load without errors. Only the "options" export is read here;
	// the module APIs won't actually be called.
	for name := range moduleShims {
		// "cbdt:http" → "__cbdt_http", "cbdt:cbdino" → "__cbdt_cbdino", etc.
		globalName := "__" + strings.ReplaceAll(name, ":", "_")
		vm.Set(globalName, vm.NewObject())
	}

	if _, err := vm.RunProgram(prog); err != nil {
		return nil, fmt.Errorf("failed to execute situation for metadata: %w", err)
	}

	workloadObj := vm.Get("__workload").ToObject(vm.Runtime)
	info, err := extractSituationInfo(vm.Runtime, workloadObj)
	if err != nil {
		return nil, fmt.Errorf("failed to extract situation info: %w", err)
	}

	return &SituationExec{
		filePath: filePath,
		program:  prog,
		info:     info,
	}, nil
}

// Info returns the situation metadata extracted during construction.
func (se *SituationExec) Info() *SituationInfo {
	return se.info
}

// Interrupt interrupts the running goja VM. Safe to call from
// another goroutine (e.g. a signal handler). No-op if Run has
// not been called yet.
func (se *SituationExec) Interrupt(v interface{}) {
	if se.vm != nil {
		se.vm.Interrupt(v)
	}
}

// Run creates a fresh goja runtime, calls setupFn to let the caller
// register modules and globals, then executes the compiled program.
// It validates that the run-time metadata matches the initial metadata,
// resolves parameters, and invokes the default export function.
func (se *SituationExec) Run(args map[string]string, setupFn SetupFn) error {
	vm := jsexec.NewRuntime()
	se.vm = vm

	// Let the caller set up modules and globals.
	jsexec.SetupConsole(vm)
	jsexec.SetupCodedError(vm)
	setupFn(vm)

	// Re-execute the compiled bytecode in the fresh runtime.
	_, err := vm.RunProgram(se.program)
	if err != nil {
		return fmt.Errorf("js execution error: %w", err)
	}

	workloadObj := vm.Get("__workload").ToObject(vm.Runtime)

	// Extract situation info and validate it matches the initial metadata.
	runInfo, err := extractSituationInfo(vm.Runtime, workloadObj)
	if err != nil {
		return fmt.Errorf("failed to extract situation info: %w", err)
	}

	if err := validateSituationInfo(se.info, runInfo); err != nil {
		return fmt.Errorf("situation metadata mismatch: %w", err)
	}

	// Resolve parameters: apply CLI args, enforce required params.
	resolvedOpts, err := resolveParams(vm.Runtime, se.info, args)
	if err != nil {
		return err
	}

	// Get the default export function.
	defaultExport := workloadObj.Get("default")
	defaultFn, ok := goja.AssertFunction(defaultExport)
	if !ok {
		return fmt.Errorf("situation file does not have a default export function")
	}

	// Invoke the default export function with resolved options.
	_, err = defaultFn(goja.Undefined(), resolvedOpts)
	if err != nil {
		return fmt.Errorf("situation execution error: %w", err)
	}

	return nil
}

// gojaToNative converts a goja value to its natural Go representation:
// JS integers → int, JS floats → float64, JS strings → string.
func gojaToNative(v goja.Value) interface{} {
	exported := v.Export()
	switch n := exported.(type) {
	case int64:
		return int(n)
	default:
		return exported
	}
}

// extractSituationInfo reads name, description, parameters, and workloads
// from the exported "options" object.
func extractSituationInfo(vm *goja.Runtime, workloadObj *goja.Object) (*SituationInfo, error) {
	optsVal := workloadObj.Get("options")
	if optsVal == nil || goja.IsUndefined(optsVal) {
		return nil, fmt.Errorf("situation file does not export 'options'")
	}

	optsObj := optsVal.ToObject(vm)
	info := &SituationInfo{
		Params:    make(map[string]SituationParam),
		Workloads: make(map[string]SituationWorkload),
	}

	if err := checkUnexpectedKeys(optsObj, "options", []string{
		"name", "description", "parameters", "workloads",
	}); err != nil {
		return nil, err
	}

	if v := optsObj.Get("name"); v != nil && !goja.IsUndefined(v) {
		info.Name = v.String()
	}

	if v := optsObj.Get("description"); v != nil && !goja.IsUndefined(v) {
		info.Description = v.String()
	}

	paramsVal := optsObj.Get("parameters")
	if paramsVal != nil && !goja.IsUndefined(paramsVal) {
		paramsObj := paramsVal.ToObject(vm)
		for _, key := range paramsObj.Keys() {
			paramObj := paramsObj.Get(key).ToObject(vm)
			param := SituationParam{}

			if err := checkUnexpectedKeys(paramObj, fmt.Sprintf("options.parameters.%s", key), []string{
				"type", "default", "choices",
			}); err != nil {
				return nil, err
			}

			if t := paramObj.Get("type"); t != nil && !goja.IsUndefined(t) {
				param.Type = t.String()
			}

			if d := paramObj.Get("default"); d != nil && !goja.IsUndefined(d) {
				param.Default = gojaToNative(d)
			}

			if ch := paramObj.Get("choices"); ch != nil && !goja.IsUndefined(ch) {
				choicesObj := ch.ToObject(vm)
				lengthVal := choicesObj.Get("length")
				if lengthVal != nil && !goja.IsUndefined(lengthVal) {
					length := int(lengthVal.ToInteger())
					for i := 0; i < length; i++ {
						elem := choicesObj.Get(fmt.Sprintf("%d", i))
						if elem != nil && !goja.IsUndefined(elem) {
							param.Choices = append(param.Choices, gojaToNative(elem))
						}
					}
				}
			}

			info.Params[key] = param
		}
	}

	workloadsVal := optsObj.Get("workloads")
	if workloadsVal != nil && !goja.IsUndefined(workloadsVal) {
		workloadsObj := workloadsVal.ToObject(vm)
		for _, name := range workloadsObj.Keys() {
			elemVal := workloadsObj.Get(name)
			if elemVal == nil || goja.IsUndefined(elemVal) {
				continue
			}

			elemObj := elemVal.ToObject(vm)
			wl := SituationWorkload{
				Scoring: make(map[string]WorkloadPhaseScoring),
			}

			elemPath := fmt.Sprintf("options.workloads.%s", name)
			if err := checkUnexpectedKeys(elemObj, elemPath, []string{
				"path", "concurrency", "rate", "scoring",
			}); err != nil {
				return nil, err
			}

			if v := elemObj.Get("path"); v != nil && !goja.IsUndefined(v) {
				wl.Path = v.String()
			}

			if v := elemObj.Get("concurrency"); v != nil && !goja.IsUndefined(v) {
				wl.Concurrency = int(v.ToInteger())
			}

			if v := elemObj.Get("rate"); v != nil && !goja.IsUndefined(v) {
				wl.Rate = v.ToFloat()
			}

			scoringVal := elemObj.Get("scoring")
			if scoringVal != nil && !goja.IsUndefined(scoringVal) {
				scoringObj := scoringVal.ToObject(vm)
				for _, phase := range scoringObj.Keys() {
					phaseObj := scoringObj.Get(phase).ToObject(vm)
					scoring := WorkloadPhaseScoring{}

					phasePath := fmt.Sprintf("%s.scoring.%s", elemPath, phase)
					if err := checkUnexpectedKeys(phaseObj, phasePath, []string{
						"min_success_rate", "max_success_rate", "min_ops_per_second",
					}); err != nil {
						return nil, err
					}

					if v := phaseObj.Get("min_success_rate"); v != nil && !goja.IsUndefined(v) {
						f := v.ToFloat()
						scoring.MinSuccessRate = &f
					}

					if v := phaseObj.Get("max_success_rate"); v != nil && !goja.IsUndefined(v) {
						f := v.ToFloat()
						scoring.MaxSuccessRate = &f
					}

					if v := phaseObj.Get("min_ops_per_second"); v != nil && !goja.IsUndefined(v) {
						f := v.ToFloat()
						scoring.MinOpsPerSecond = &f
					}

					wl.Scoring[phase] = scoring
				}
			}

			info.Workloads[name] = wl
		}
	}

	return info, nil
}

// checkUnexpectedKeys returns an error if obj contains any keys not in allowed.
func checkUnexpectedKeys(obj *goja.Object, path string, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}
	for _, key := range obj.Keys() {
		if _, ok := allowedSet[key]; !ok {
			return fmt.Errorf("unexpected field %q in %s", key, path)
		}
	}
	return nil
}

// resolveParams merges CLI-provided args with parameter defaults.
func resolveParams(vm *goja.Runtime, info *SituationInfo, args map[string]string) (goja.Value, error) {
	resolved := vm.NewObject()

	var missing []string
	for name, param := range info.Params {
		var val interface{}
		var hasVal bool

		if v, ok := args[name]; ok {
			// CLI args are always strings; parse to the correct Go type.
			parsed, err := parseParamValue(v, param.Type)
			if err != nil {
				return nil, fmt.Errorf("parameter %q: %w", name, err)
			}
			val = parsed
			hasVal = true
		} else if param.Default != nil {
			val = param.Default
			hasVal = true
		} else {
			missing = append(missing, name)
		}

		if hasVal {
			if len(param.Choices) > 0 {
				valid := false
				for _, c := range param.Choices {
					if valuesEqual(c, val) {
						valid = true
						break
					}
				}
				if !valid {
					return nil, fmt.Errorf("invalid value %v for parameter %q (choices: %v)", val, name, param.Choices)
				}
			}
			resolved.Set(name, val)
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required parameter(s): %v", missing)
	}

	return resolved, nil
}

// parseParamValue converts a CLI string argument to the appropriate Go type
// based on the parameter's declared type.
func parseParamValue(s string, paramType string) (interface{}, error) {
	switch paramType {
	case "number":
		// Try integer first, then float.
		if n, err := strconv.Atoi(s); err == nil {
			return n, nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", s, err)
		}
		return f, nil
	default:
		return s, nil
	}
}

// valuesEqual compares two parameter values, handling numeric type coercion
// (e.g. int(5) == float64(5.0)).
func valuesEqual(a, b interface{}) bool {
	if a == b {
		return true
	}
	// Coerce both to float64 for numeric comparison.
	af, aOk := toFloat64(a)
	bf, bOk := toFloat64(b)
	if aOk && bOk {
		return af == bf
	}
	return false
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case float64:
		if math.IsNaN(n) {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// validateSituationInfo checks that the run-time metadata matches the
// metadata extracted during construction.
func validateSituationInfo(initial, current *SituationInfo) error {
	if initial.Name != current.Name {
		return fmt.Errorf("name changed: %q vs %q", initial.Name, current.Name)
	}
	if initial.Description != current.Description {
		return fmt.Errorf("description changed: %q vs %q", initial.Description, current.Description)
	}
	if !reflect.DeepEqual(initial.Params, current.Params) {
		return fmt.Errorf("parameters changed between runs")
	}
	if !reflect.DeepEqual(initial.Workloads, current.Workloads) {
		return fmt.Errorf("workloads changed between runs")
	}
	return nil
}
