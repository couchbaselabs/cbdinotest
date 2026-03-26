// Global Workload controller available in situation scripts.

/** Workload entry in the array passed to `Workload.start()`. */
interface WorkloadStartEntry {
  /** Path to the workload script (e.g. "workloads/user-update-get.js"). */
  workload: string;
  /** Number of concurrent workers for this workload. */
  concurrency?: number;
}

/** Global Workload controller. */
interface WorkloadGlobal {
  /** Set up the worker and workloads (indexes, seed data, etc.). */
  setup(setupOpts: any): void;
  /** Start dispatching workload operations. Must call setup() first. */
  start(): void;
  /** Stop all running workloads. */
  stop(): void;
}

declare const Workload: WorkloadGlobal;
