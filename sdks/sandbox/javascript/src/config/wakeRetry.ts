// Copyright 2026 The OpenSandbox Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { SandboxWakeTimeoutException } from "../core/exceptions.js";

/**
 * OSEP-0024 wake-timeout marker. The gateway strips this header from every
 * upstream response, so its presence on a 503 proves gateway origin: the
 * request was parked and never forwarded, which is what makes any-method
 * retry safe.
 */
export const WAKE_MARKER_HEADER = "OpenSandbox-Wake";
export const WAKE_MARKER_VALUE = "expired";

/**
 * The gateway emits `Retry-After: 1` on wake-timeout responses. Used as the
 * fallback wait when a marked response arrives without the header.
 */
export const WAKE_DEFAULT_RETRY_AFTER_MS = 1000;

/**
 * Fixed operational ceiling for server-supplied `Retry-After` waits. Not
 * configurable: ignoring a server's back-pressure signal is never a safe
 * default, and the 60s guard only defends against a pathological header.
 */
export const RETRY_AFTER_CAP_MS = 60_000;

// Overflow guard for numeric `Retry-After` values, independent of the
// operational cap.
const MAX_RETRY_AFTER_MS = 100 * 365 * 24 * 3600 * 1000;

/**
 * Policy for auto-retrying gateway wake-timeout responses (OSEP-0024).
 */
export interface WakeRetryOptions {
  /**
   * Auto-retry gateway wake-timeout responses: a 503 carrying the
   * `OpenSandbox-Wake: expired` marker proves the request was never
   * forwarded, so the original request is resent for any method after
   * waiting per `Retry-After` — provided the body is replayable. Bare 503
   * responses are never affected by this knob.
   */
  enabled: boolean;
  /**
   * Maximum wake-timeout responses (marked 503) consumed before the wake
   * retry gives up and throws {@link SandboxWakeTimeoutException}.
   */
  maxAttempts: number;
  /**
   * Wall-clock cap in seconds spanning all wake cycles of one logical
   * request: waits, retries, and request time. Wake retries never reset it;
   * a next `Retry-After` that does not fit the remaining budget ends the
   * wake retry with a typed exception. `null` removes the time bound (the
   * attempt count still applies).
   */
  overallDeadlineSeconds: number | null;
}

/**
 * Parse a `Retry-After` header (delta-seconds or HTTP-date) into
 * milliseconds. Past dates and negative deltas normalize to zero;
 * unparseable values return `null`.
 */
export function parseRetryAfterMs(
  headerValue: string | null | undefined,
  now: number = Date.now(),
): number | null {
  if (headerValue == null) return null;
  const raw = headerValue.trim();
  if (!raw) return null;
  if (/^[+-]?\d+$/.test(raw)) {
    const seconds = Number(raw);
    if (seconds <= 0) return 0;
    return Math.min(seconds * 1000, MAX_RETRY_AFTER_MS);
  }
  const date = Date.parse(raw);
  if (Number.isNaN(date)) return null;
  const delta = date - now;
  return delta < 0 ? 0 : delta;
}

/**
 * Effective wake retry wait for a marked response: the parsed `Retry-After`
 * (delta-seconds or HTTP-date) capped at the operational ceiling, or the
 * 1-second fallback when the header is missing or unparseable.
 */
export function resolveWakeRetryAfterMs(
  response: Response,
  now: number = Date.now(),
): number {
  const raw = parseRetryAfterMs(response.headers.get("Retry-After"), now);
  return Math.min(raw ?? WAKE_DEFAULT_RETRY_AFTER_MS, RETRY_AFTER_CAP_MS);
}

/**
 * Whether the response is the OSEP-0024 wake-timeout answer.
 *
 * Recognition is header-only on a 503: the gateway strips
 * `OpenSandbox-Wake` from every upstream response, so a matching marker
 * proves the gateway parked the request and never forwarded it. The JSON
 * `code` in the body is documentation for humans, not a recognition
 * signal — a forged body without the header follows the SDK's existing
 * (bare 503) rules untouched.
 */
export function isWakeTimeoutResponse(response: Response): boolean {
  return (
    response.status === 503 &&
    response.headers.get(WAKE_MARKER_HEADER) === WAKE_MARKER_VALUE
  );
}

interface Replay {
  body?: Exclude<NonNullable<RequestInit["body"]>, ReadableStream>;
  requestInit?: {
    url: string;
    method: string;
    headers: [string, string][];
  };
}

/**
 * Capture a replayable copy of the request body.
 *
 * In-memory bodies (strings, buffers, blobs, URLSearchParams) can be
 * re-dispatched as-is; streaming bodies (ReadableStream) and FormData can
 * only be consumed once, so requests carrying them are never retried.
 * Returns `null` when the request cannot be replayed.
 */
async function captureReplay(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Replay | null> {
  const body = init?.body;
  if (body == null) {
    if (typeof Request !== "undefined" && input instanceof Request && input.body != null) {
      // Buffer the Request's body via a clone taken before the first send.
      try {
        const buffered = await input.clone().arrayBuffer();
        const headers: [string, string][] = [];
        input.headers.forEach((value, key) => headers.push([key, value]));
        return {
          body: buffered,
          requestInit: {
            url: input.url,
            method: input.method,
            headers,
          },
        };
      } catch {
        return null;
      }
    }
    return { body: undefined };
  }
  if (
    typeof body === "string" ||
    body instanceof ArrayBuffer ||
    ArrayBuffer.isView(body) ||
    body instanceof Blob ||
    body instanceof URLSearchParams
  ) {
    return { body };
  }
  return null;
}

function sleepWithAbort(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(signal.reason ?? new Error("Aborted"));
      return;
    }
    const done = () => {
      signal?.removeEventListener("abort", onAbort as EventListener);
      clearTimeout(timer);
      resolve();
    };
    const onAbort = () => {
      clearTimeout(timer);
      reject(signal!.reason ?? new Error("Aborted"));
    };
    const timer = setTimeout(done, ms);
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}

/**
 * Wrap a fetch with the OSEP-0024 wake retry contract.
 *
 * On a marked wake-timeout response the original request is resent after
 * waiting per `Retry-After` (any method, replayable bodies only), until the
 * sandbox is `Running` or the policy budget is exhausted; exhaustion throws
 * {@link SandboxWakeTimeoutException} while the server-side resume continues
 * in the background. Every other response — including a bare 503 — is
 * returned untouched.
 */
export function createWakeRetryFetch(opts: {
  baseFetch: typeof fetch;
  wakeRetry: WakeRetryOptions;
}): typeof fetch {
  const baseFetch = opts.baseFetch;
  const policy = opts.wakeRetry;

  return async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    if (!policy.enabled) {
      return baseFetch(input, init);
    }
    // Callers may carry the abort signal on the Request itself (openapi-fetch
    // constructs a Request and passes only extra fields as init), so resolve
    // the signal the same way createTimedFetch does.
    const signal: AbortSignal | undefined =
      init?.signal ??
      (typeof Request !== "undefined" && input instanceof Request
        ? input.signal
        : undefined) ??
      undefined;
    const replay = await captureReplay(input, init);
    const startedAt = Date.now();
    let wakeAttemptsUsed = 0;

    // Dispatch one attempt. Replays rebuild a Request when the original
    // input was a Request carrying its own body.
    const dispatch = async (): Promise<Response> => {
      if (replay == null) {
        return baseFetch(input, { ...init, signal });
      }
      if (replay.requestInit) {
        const r = replay.requestInit;
        const retryRequest = new Request(r.url, {
          method: r.method,
          headers: r.headers,
          body: replay.body,
          signal,
        });
        const retryInit: RequestInit = { ...init, signal };
        delete retryInit.body;
        return baseFetch(retryRequest, retryInit);
      }
      return baseFetch(input, { ...init, signal, body: replay.body });
    };

    while (true) {
      const res = await dispatch();
      if (!isWakeTimeoutResponse(res)) {
        return res;
      }
      // Without a replayable body the request cannot be resent; follow the
      // SDK's existing (bare 503) rules and return the marked response as-is.
      if (replay == null) {
        return res;
      }

      wakeAttemptsUsed += 1;
      const elapsedMs = Date.now() - startedAt;
      const remainingMs =
        policy.overallDeadlineSeconds == null
          ? null
          : Math.max(0, policy.overallDeadlineSeconds * 1000 - elapsedMs);

      const retryAfterMs = resolveWakeRetryAfterMs(res, Date.now());

      // Budget checks bound retries, not a committed resume: the
      // server-side restore is never canceled by giving up here.
      if (wakeAttemptsUsed >= policy.maxAttempts || (remainingMs != null && retryAfterMs > remainingMs)) {
        const rawBody = await res.text().catch(() => undefined);
        throw new SandboxWakeTimeoutException({
          message:
            "sandbox wake timeout: restore did not finish within the wake retry budget; the request was never forwarded and is safe to retry",
          attempts: wakeAttemptsUsed,
          waitedMs: elapsedMs,
          requestId: res.headers.get("x-request-id") ?? undefined,
          rawBody,
        });
      }

      // Release the marked response's connection before waiting.
      void res.body?.cancel().catch(() => undefined);
      await sleepWithAbort(retryAfterMs, signal);
    }
  };
}
