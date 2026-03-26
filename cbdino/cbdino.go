package cbdino

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// ControllerOptions holds optional parameters for creating a Controller.
type ControllerOptions struct {
	// ReuseCbdcID controls cluster reuse behavior:
	//   - nil: normal allocate/remove (default)
	//   - pointer to "": first allocate proceeds normally but skips removal;
	//     the allocated cluster ID is written back through the pointer
	//   - pointer to "ID": reuse that cluster via modify instead of allocate
	ReuseCbdcID *string
}

// Controller provides an interface for executing cbdinocluster commands
// via the binary. It handles finding the binary and executing commands.
type Controller struct {
	binaryPath      string
	createdClusters []string
	reuseCbdcID     *string
}

// NewController creates a new Controller, locating the cbdinocluster binary.
// It searches in the following order:
//  1. CBDINOCLUSTER_BIN environment variable
//  2. Sibling repo path ../cbdinocluster/cbdinocluster (relative to this module)
//  3. PATH via exec.LookPath
func NewController(opts ControllerOptions) (*Controller, error) {
	binPath, err := findBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to find cbdinocluster binary: %w", err)
	}

	var reuseCbdcID *string
	if opts.ReuseCbdcID != nil {
		reuseCbdcIDStr := *opts.ReuseCbdcID
		reuseCbdcID = &reuseCbdcIDStr
	}

	return &Controller{
		binaryPath:  binPath,
		reuseCbdcID: reuseCbdcID,
	}, nil
}

// BinaryPath returns the resolved path to the cbdinocluster binary.
func (c *Controller) BinaryPath() string {
	return c.binaryPath
}

// findBinary searches for the cbdinocluster binary in standard locations.
func findBinary() (string, error) {
	// 1. Check CBDINOCLUSTER_BIN env var
	if envPath := os.Getenv("CBDINOCLUSTER_BIN"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath, nil
		}
	}

	// 2. Check sibling repo path relative to working directory
	directPaths := []string{
		"./cbdinocluster",
	}
	for _, relPath := range directPaths {
		absPath, err := filepath.Abs(relPath)
		if err == nil {
			if _, err := os.Stat(absPath); err == nil {
				return absPath, nil
			}
		}
	}

	// 3. Check PATH
	if pathBin, err := exec.LookPath("cbdinocluster"); err == nil {
		return pathBin, nil
	}

	return "", fmt.Errorf("cbdinocluster binary not found; set CBDINOCLUSTER_BIN or ensure it is in PATH")
}

// run executes cbdinocluster with the given arguments and returns stdout.
// Stderr is streamed line-by-line to the log with a [CBDC] prefix.
func (c *Controller) run(args ...string) (string, error) {
	cmd := exec.Command(c.binaryPath, args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	fmt.Printf("[CBDC] > cbdinocluster %s\n", strings.Join(args, " "))

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("cbdinocluster %s failed to start: %w",
			strings.Join(args, " "), err)
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	// Stream stdout line-by-line with [CBDC] prefix while capturing it
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Printf("[CBDC] %s\n", line)
			stdoutBuf.WriteString(line)
			stdoutBuf.WriteByte('\n')
		}
	}()

	// Stream stderr line-by-line with [CBDC] prefix
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			line = stripTimestamp(line)
			fmt.Printf("[CBDC] %s\n", line)
			stderrBuf.WriteString(line)
			stderrBuf.WriteByte('\n')
		}
	}()

	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("cbdinocluster %s failed: %w\nstderr: %s",
			strings.Join(args, " "), err, stderrBuf.String())
	}

	fmt.Printf("[CBDC] > execution complete\n")

	return strings.TrimSpace(stdoutBuf.String()), nil
}

// trackCluster records a cluster ID as having been created by this controller.
func (c *Controller) trackCluster(clusterID string) {
	c.createdClusters = append(c.createdClusters, clusterID)
}

// untrackCluster removes a cluster ID from the tracked list.
func (c *Controller) untrackCluster(clusterID string) {
	for i, id := range c.createdClusters {
		if id == clusterID {
			c.createdClusters = append(c.createdClusters[:i], c.createdClusters[i+1:]...)
			return
		}
	}
}

// ReusableCbdcID returns the current reuse cluster ID, or nil if reuse is not active.
func (c *Controller) ReusableCbdcID() string {
	if c.reuseCbdcID == nil {
		return ""
	}
	return *c.reuseCbdcID
}

// CreatedClusterIDs returns the list of cluster IDs created by this controller.
func (c *Controller) CreatedClusterIDs() []string {
	out := make([]string, len(c.createdClusters))
	copy(out, c.createdClusters)
	return out
}

// RemoveAllCreated removes all clusters that were created by this controller instance.
// It attempts to remove every cluster and returns a combined error if any removals fail.
func (c *Controller) RemoveAllCreated() error {
	var errs []error
	for _, clusterID := range c.createdClusters {
		if err := c.Remove(clusterID); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove cluster %s: %w", clusterID, err))
		}
	}
	c.createdClusters = nil

	if len(errs) > 0 {
		return fmt.Errorf("errors removing created clusters: %v", errs)
	}
	return nil
}

// runJSON executes cbdinocluster with --json prepended and decodes the
// JSON output into the provided destination.
func (c *Controller) runJSON(dest interface{}, args ...string) error {
	fullArgs := append([]string{"--json"}, args...)
	output, err := c.run(fullArgs...)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(output), dest); err != nil {
		return fmt.Errorf("failed to parse JSON output: %w\nraw output: %s", err, output)
	}

	return nil
}

// timestampRegex matches zap-style timestamps at the start of a line,
// e.g. "2026-03-12T12:21:30.471-0700\t"
var timestampRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}[+-]\d{4}\t`)

// stripTimestamp removes a leading zap-style timestamp and its trailing tab from a line.
func stripTimestamp(line string) string {
	return timestampRegex.ReplaceAllString(line, "")
}
