// Global utility functions available in situation scripts.

/** Global utility functions. */
interface UtilsGlobal {
  /** Sleep for the specified number of milliseconds (blocking). */
  sleep(ms: number): void;
  /** Parse a YAML string and return the resulting JS object. */
  parseYaml(yamlString: string): unknown;
}

declare const Utils: UtilsGlobal;
