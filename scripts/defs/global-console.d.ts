// Global console object (subset) available in all script environments.

interface Console {
  /** Log one or more values to stdout. */
  log(...args: unknown[]): void;
}

declare const console: Console;
