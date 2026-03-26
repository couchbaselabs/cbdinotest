// Type definitions for the cbdt:http module.
// Provides HTTP methods for making requests to external services.

/** Options for HTTP request headers, body, and timeout. */
interface HttpRequestOptions {
  /** HTTP headers to include in the request. */
  headers?: Record<string, string>;
  /** Request body as a string (typically JSON). */
  body?: string;
  /** Request timeout in milliseconds. Defaults to 5000 (5s). */
  timeout?: number;
}

/** HTTP response returned by all cbdt:http methods. */
interface HttpResponse {
  /** HTTP status code. */
  status: number;
  /** HTTP status text. */
  statusText: string;
  /** Response headers. */
  headers: Record<string, string>;
  /** Response body as a string. */
  body: string;
}

/** The cbdt:http module. */
interface HttpModule {
  /** Perform an HTTP GET request. */
  get(url: string, opts?: HttpRequestOptions): HttpResponse;
  /** Perform an HTTP POST request. */
  post(url: string, opts?: HttpRequestOptions): HttpResponse;
  /** Perform an HTTP PUT request. */
  put(url: string, opts?: HttpRequestOptions): HttpResponse;
  /** Perform an HTTP DELETE request. */
  delete(url: string, opts?: HttpRequestOptions): HttpResponse;
}

declare module "cbdt:http" {
  const http: HttpModule;
  export default http;
}
