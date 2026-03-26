package jsexec

import (
	"fmt"

	"github.com/dop251/goja"
)

// ExtractFunc extracts a callable function from a Goja object by key.
func ExtractFunc(obj *goja.Object, name string) (goja.Callable, error) {
	val := obj.Get(name)
	if val == nil || goja.IsUndefined(val) {
		return nil, fmt.Errorf("%q is not defined", name)
	}

	fn, ok := goja.AssertFunction(val)
	if !ok {
		return nil, fmt.Errorf("%q is not a function", name)
	}

	return fn, nil
}

// IsInterrupt checks whether an error is a Goja interrupt.
func IsInterrupt(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*goja.InterruptedError)
	return ok
}
