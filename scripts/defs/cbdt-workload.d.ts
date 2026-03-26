// Type definitions for the cbdt:workload module.
// Provides workload runner creation and management.

/** A workload runner instance created via the cbdt:workload module. */
interface WorkloadRunner {
  /** Set the active workload type by name (e.g. "kv-update-get"). */
  setWorkload(name: string): void;
  /** Call the setup endpoint with optional configuration. */
  setup(opts?: unknown): void;
  /** Call the shutdown endpoint. */
  shutdown(): void;
  /** Start executing the workload with the given concurrency (default 4). */
  start(concurrency?: number): void;
  /** Stop the workload and return the total operation count. */
  stop(): number;
  /** Check whether the workload is currently running. */
  isRunning(): boolean;
  /** Get the current completed operation count. */
  opsCount(): number;
}

/** The cbdt:workload module. */
interface WorkloadModule {
  /** Create a new workload runner targeting the given endpoint. */
  create(endpoint: string): WorkloadRunner;
}

declare module "cbdt:workload" {
  const workload: WorkloadModule;
  export default workload;
}
