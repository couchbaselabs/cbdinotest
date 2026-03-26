// Type definitions for the cbdt:metrics module.
// Provides event recording for metrics collection.

/** The cbdt:metrics module. */
interface MetricsModule {
  /** Record a named event. */
  recordEvent(name: string): void;
}

declare module "cbdt:metrics" {
  const metrics: MetricsModule;
  export default metrics;
}
