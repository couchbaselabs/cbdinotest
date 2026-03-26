package jsexec

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

func consoleLog(call goja.FunctionCall) goja.Value {
	args := make([]string, len(call.Arguments))
	for i, arg := range call.Arguments {
		args[i] = arg.String()
	}
	fmt.Printf("[SITUATION:LOG] %s\n", strings.Join(args, " "))
	return goja.Undefined()
}

// SetupConsole registers the console.log global on the runtime.
func SetupConsole(vm *Runtime) {
	console := vm.NewObject()
	console.Set("log", consoleLog)
	vm.Set("console", console)
}
