package workloadexec

import (
	"fmt"
)

// WorkloadInfo holds metadata extracted from the workload's options export.
type WorkloadInfo struct {
	Name        string
	Description string
}

// Workload holds a compiled workload script and provides access to
// its metadata and lifecycle functions (setup, default, teardown).
// Setup/teardown are called once by the WorkloadRunner; the compiled
// program is shared with WorkloadPool for cloning instances.
type Workload struct {
	exec *WorkloadExec
}

// NewWorkload bundles and compiles the workload script at filePath.
func NewWorkload(filePath string) (*Workload, error) {
	exec, err := NewWorkloadExec(filePath)
	if err != nil {
		return nil, err
	}

	return &Workload{exec: exec}, nil
}

// Info returns the workload metadata extracted during compilation.
func (w *Workload) Info() WorkloadInfo {
	return w.exec.Info()
}

// Setup sets the worker URI on the lifecycle exec and calls the
// workload's exported setup() function once. The setup data is
// stored internally on the exec and will be automatically carried
// to clones.
func (w *Workload) Setup(workerURI string) error {
	w.exec.SetWorkerURI(workerURI)

	if err := w.exec.Setup(); err != nil {
		return fmt.Errorf("workload setup failed: %w", err)
	}

	return nil
}

// Teardown calls the workload's exported teardown() function once,
// passing the internally stored setup data.
func (w *Workload) Teardown() error {
	return w.exec.Teardown()
}
