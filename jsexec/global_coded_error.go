package jsexec

import (
	"fmt"

	"github.com/dop251/goja"
)

// codedErrorClass is the JS source for the CodedError class.
// Defined in Go so the runtime owns the constructor and can detect
// instances via instanceof checks.
const codedErrorClass = `
class CodedError extends Error {
	constructor(code, details) {
		super(code + ': ' + details);
		this.name = 'CodedError';
		this.code = code;
		this.details = details;
	}
}
`

// codedErrorProgram is the pre-compiled JS program for the CodedError class.
var codedErrorProgram = goja.MustCompile("CodedError", codedErrorClass, false)

// SetupCodedError registers the CodedError class as a global
// constructor on the runtime. Scripts can then use
// `new CodedError(code, details)` and the Go side can detect
// these via IsCodedError.
func SetupCodedError(vm *Runtime) {
	_, err := vm.RunProgram(codedErrorProgram)
	if err != nil {
		panic("failed to register CodedError class: " + err.Error())
	}
}

// IsCodedError checks whether a Goja value is an instance of the
// CodedError class and extracts the code and details if so.
func IsCodedError(vm *goja.Runtime, val goja.Value) (code string, details string, ok bool) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return "", "", false
	}

	obj := val.ToObject(vm)
	nameVal := obj.Get("name")
	if nameVal == nil || goja.IsUndefined(nameVal) || nameVal.String() != "CodedError" {
		return "", "", false
	}

	codeVal := obj.Get("code")
	detailsVal := obj.Get("details")
	if codeVal == nil || goja.IsUndefined(codeVal) {
		return "", "", false
	}

	code = codeVal.String()
	if detailsVal != nil && !goja.IsUndefined(detailsVal) {
		details = detailsVal.String()
	}

	return code, details, true
}

// ThrowCodedError creates and returns a CodedError instance for panicking.
func ThrowCodedError(vm *goja.Runtime, code, details string) goja.Value {
	ctor := vm.Get("CodedError")
	if ctor == nil || goja.IsUndefined(ctor) {
		return vm.NewGoError(fmt.Errorf("%s: %s", code, details))
	}

	ctorFn, ok := goja.AssertFunction(ctor)
	if !ok {
		return vm.NewGoError(fmt.Errorf("%s: %s", code, details))
	}

	errVal, err := ctorFn(goja.Undefined(), vm.ToValue(code), vm.ToValue(details))
	if err != nil {
		return vm.NewGoError(fmt.Errorf("%s: %s", code, details))
	}

	return errVal
}
