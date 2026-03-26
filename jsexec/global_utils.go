package jsexec

import (
	"fmt"
	"time"

	"github.com/dop251/goja"
	"gopkg.in/yaml.v3"
)

func utilsSleep(vm *Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		ms := call.Argument(0).ToInteger()
		d := time.Duration(ms) * time.Millisecond
		fmt.Printf("[SITUATION] sleeping for %s\n", d)

		timer := time.NewTimer(d)
		defer timer.Stop()

		select {
		case <-timer.C:
			// sleep completed normally
		case <-vm.DoneCh():
			// interrupted
		}

		return goja.Undefined()
	}
}

func utilsParseYaml(vm *Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		yamlStr := call.Argument(0).String()

		var parsed interface{}
		if err := yaml.Unmarshal([]byte(yamlStr), &parsed); err != nil {
			panic(vm.NewGoError(fmt.Errorf("failed to parse YAML: %w", err)))
		}

		result := vm.ToValue(convertYamlValue(parsed))
		return result
	}
}

// convertYamlValue recursively converts yaml.v3 parsed values into
// types that goja can natively handle (map[string]interface{} and []interface{}).
func convertYamlValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v := range val {
			out[k] = convertYamlValue(v)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v := range val {
			out[fmt.Sprintf("%v", k)] = convertYamlValue(v)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, v := range val {
			out[i] = convertYamlValue(v)
		}
		return out
	default:
		return v
	}
}

// SetupUtils registers the Utils global (sleep, parseYaml) on the runtime.
func SetupUtils(vm *Runtime) {
	utils := vm.NewObject()
	utils.Set("sleep", utilsSleep(vm))
	utils.Set("parseYaml", utilsParseYaml(vm))
	vm.Set("Utils", utils)
}
