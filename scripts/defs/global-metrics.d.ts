// Global Metrics controller available in situation scripts.

/** Global Metrics controller. */
interface MetricsGlobal {
  /** Mark the start of a named phase for metric tracking. */
  markPhase(name: string): void;
}

declare const Metrics: MetricsGlobal;
