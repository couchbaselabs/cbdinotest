// Global CodedError class injected by the Go runtime.
// Extends Error with structured code and details fields,
// matching the MetricsError pattern from the Go metrics package.

/**
 * CodedError represents a structured error with a machine-readable
 * code and human-readable details. The Go runtime can detect instances
 * of this class and extract the code/details for metrics tracking.
 */
declare class CodedError extends Error {
    /** Machine-readable error code (e.g. "upsert_failed", "get_failed"). */
    readonly code: string;
    /** Human-readable error details. */
    readonly details: string;

    constructor(code: string, details: string);
}
