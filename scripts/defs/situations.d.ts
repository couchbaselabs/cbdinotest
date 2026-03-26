// Type definitions for cbdinotest situation scripts.
// References all globals and modules available in the situation runtime.

/// <reference path="cbdt-cbdino.d.ts" />
/// <reference path="cbdt-http.d.ts" />
/// <reference path="cbdt-workload.d.ts" />
/// <reference path="cbdt-metrics.d.ts" />
/// <reference path="global-console.d.ts" />
/// <reference path="global-workload.d.ts" />
/// <reference path="global-metrics.d.ts" />
/// <reference path="global-utils.d.ts" />
/// <reference path="global-workload-error.d.ts" />

/** The directory path of the current script. */
declare const __dirname: string;

// =============================================================================
// Situation script export shape
// =============================================================================

/** Parameter definition in the situation options export. */
interface SituationParamDef {
  /** Parameter type (e.g. "string"). */
  type: string;
  /** Optional default value. If omitted, the parameter is required. */
  default?: string;
}

/** The `options` export from a situation script. */
interface SituationOptions {
  /** Unique name for this situation. */
  name: string;
  /** Human-readable description. */
  description?: string;
  /** Parameter definitions keyed by parameter name. */
  parameters?: Record<string, SituationParamDef>;
}
