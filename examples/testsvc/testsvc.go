package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code string, details string) {
	writeJSON(w, status, map[string]string{"code": code, "details": details})
}

func handleSetup(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("testsvc: setup\n")
	w.WriteHeader(http.StatusOK)
}

func handleCleanup(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("testsvc: cleanup\n")
	w.WriteHeader(http.StatusOK)
}

func handleTest(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("testsvc: test\n")
	w.WriteHeader(http.StatusOK)
}

var (
	lastMetricsTime time.Time
	lastCPUTimeUsec int64
	lastMetricsMu   sync.Mutex
)

func cpuTimeUsec(ru *syscall.Rusage) int64 {
	return int64(ru.Utime.Sec)*1e6 +
		int64(ru.Utime.Usec) +
		int64(ru.Stime.Sec)*1e6 +
		int64(ru.Stime.Usec)
}

// rssBytes returns the resident set size in bytes.
// On Linux ru_maxrss is in kilobytes; on macOS it is already in bytes.
func rssBytes(ru *syscall.Rusage) int64 {
	if runtime.GOOS == "linux" {
		return ru.Maxrss * 1024
	}
	return ru.Maxrss
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		writeError(w, http.StatusInternalServerError, "rusage_failed", err.Error())
		return
	}

	now := time.Now()
	currentCPU := cpuTimeUsec(&rusage)

	lastMetricsMu.Lock()
	var cpuPercent float64
	if !lastMetricsTime.IsZero() {
		wallElapsed := now.Sub(lastMetricsTime).Microseconds()
		if wallElapsed > 0 {
			cpuDelta := currentCPU - lastCPUTimeUsec
			cpuPercent = float64(cpuDelta) / float64(wallElapsed) * 100.0
		}
	}
	lastMetricsTime = now
	lastCPUTimeUsec = currentCPU
	lastMetricsMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cpu_percent": cpuPercent,
		"rss_bytes":   rssBytes(&rusage),
	})
}

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/_setup", handleSetup).Methods("POST")
	r.HandleFunc("/_cleanup", handleCleanup).Methods("POST")
	r.HandleFunc("/_metrics", handleMetrics).Methods("GET")

	r.HandleFunc("/test", handleTest).Methods("GET")

	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}

	log.Printf("testsvc listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server failed: %s", err)
	}
}
