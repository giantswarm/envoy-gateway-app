import http from "k6/http";
import { check, group } from "k6";
import encoding from "k6/encoding";

// All tunables read from environment variables (set in the TestRun CRD)
// with sensible defaults so the script still works standalone.

// Infrastructure
const ENDPOINTS  = parseInt(__ENV.ENDPOINTS || "10", 10);
const BASE_DOMAIN = __ENV.BASE_DOMAIN;
const PROXY_CONTROLLER = (__ENV.PROXY_CONTROLLER || "nginx").toLowerCase();
if (PROXY_CONTROLLER !== "nginx" && PROXY_CONTROLLER !== "kong") {
  throw new Error(`PROXY_CONTROLLER must be 'nginx' or 'kong' (got: '${PROXY_CONTROLLER}')`);
}

// Basic Auth credentials — must match what the suite provisions on the
// gateways (BASIC_AUTH_USER / BASIC_AUTH_PASSWORD in the TestRun env).
const BASIC_AUTH_USER     = __ENV.BASIC_AUTH_USER     || "testuser";
const BASIC_AUTH_PASSWORD = __ENV.BASIC_AUTH_PASSWORD || "testpassword";
const VALID_CREDENTIALS   = encoding.b64encode(`${BASIC_AUTH_USER}:${BASIC_AUTH_PASSWORD}`);
const WRONG_CREDENTIALS   = encoding.b64encode("wrong-user:wrong-password");

// Scenario timing & load shape
const SCENARIO_DURATION_SECONDS  = parseInt(__ENV.SCENARIO_DURATION_SECONDS  || "1200", 10); // 20m
const WAIT_BETWEEN_SCENARIOS     = parseInt(__ENV.WAIT_BETWEEN_SCENARIOS     || "300",  10); // 5m
const PEAK_HTTP_RPS              = parseInt(__ENV.PEAK_HTTP_RPS              || "50",   10); // target HTTP req/s
const RAMP_STEP_HTTP_RPS         = parseInt(__ENV.RAMP_STEP_HTTP_RPS         || "0",    10); // 0 = no ramp
const RAMP_STEP_DURATION_SECONDS = parseInt(__ENV.RAMP_STEP_DURATION_SECONDS || "300",  10);
const PRE_ALLOCATED_VUS          = parseInt(__ENV.PRE_ALLOCATED_VUS          || "50",   10);
const MAX_VUS                    = parseInt(__ENV.MAX_VUS                    || "150",  10);
const GRACEFUL_STOP              = __ENV.GRACEFUL_STOP || "30s";

// Each iteration of basicAuthFlow issues this many HTTP requests (valid, wrong,
// missing). The *_HTTP_RPS budgets are HTTP req/s, so convert them into a k6
// iteration (arrival) rate by dividing — mirrors the "basic" suite's
// AVG_HTTP_PER_ITERATION handling so load levels stay comparable.
const REQUESTS_PER_ITERATION = 3;
const httpRpsToIterRate = (httpRps) =>
  Math.max(1, Math.round(httpRps / REQUESTS_PER_ITERATION));
const PEAK_ITERATION_RATE      = httpRpsToIterRate(PEAK_HTTP_RPS);
const RAMP_STEP_ITERATION_RATE = RAMP_STEP_HTTP_RPS > 0 ? httpRpsToIterRate(RAMP_STEP_HTTP_RPS) : 0;

// SLO thresholds — align with giantswarm/giantswarm#35147 recommendations.
const SLO_P95_LATENCY_MS = __ENV.SLO_P95_LATENCY_MS || "500";
const SLO_P99_LATENCY_MS = __ENV.SLO_P99_LATENCY_MS || "1000";
const SLO_ERROR_RATE     = __ENV.SLO_ERROR_RATE      || "0.001";  // 0.1%
const SLO_CHECKS_RATE    = __ENV.SLO_CHECKS_RATE     || "0.95";

// Treat 401 as an expected status so the intentional "wrong"/"missing"
// credential requests do not inflate http_req_failed. Mirrors the upstream
// use-case's `http.expectedStatuses({ min: 200, max: 399 }, 401)`.
const expectedStatuses = http.expectedStatuses({ min: 200, max: 399 }, 401);

function pickEnvoyBaseUrl() {
  const n = Math.floor(Math.random() * ENDPOINTS);
  return `https://onlineboutique.loadtesting-${n}.${BASE_DOMAIN}`;
}

function pickNginxBaseUrl() {
  const n = Math.floor(Math.random() * ENDPOINTS);
  return `https://nginx-onlineboutique-${n}.loadtesting.${BASE_DOMAIN}`;
}

function kongBaseUrl() {
  // Kong runs as a Gateway API implementation here: the chart exposes a single
  // HTTPRoute host (kong-onlineboutique.loadtesting.<base>), with no per-endpoint
  // fan-out like the Envoy/nginx sides, so all kong traffic targets that host.
  return `https://kong-onlineboutique.loadtesting.${BASE_DOMAIN}`;
}

function buildScenarioConfig() {
  const base = {
    timeUnit: "1s",
    preAllocatedVUs: PRE_ALLOCATED_VUS,
    maxVUs: MAX_VUS,
    gracefulStop: GRACEFUL_STOP,
  };
  if (RAMP_STEP_ITERATION_RATE > 0 && PEAK_ITERATION_RATE > RAMP_STEP_ITERATION_RATE) {
    const numSteps = Math.ceil(PEAK_ITERATION_RATE / RAMP_STEP_ITERATION_RATE);
    const stages = [];
    for (let i = 1; i <= numSteps; i++) {
      const target = Math.min(i * RAMP_STEP_ITERATION_RATE, PEAK_ITERATION_RATE);
      stages.push({ target, duration: `${RAMP_STEP_DURATION_SECONDS}s` });
    }
    const rampSeconds = numSteps * RAMP_STEP_DURATION_SECONDS;
    const holdSeconds = Math.max(0, SCENARIO_DURATION_SECONDS - rampSeconds);
    if (holdSeconds > 0) {
      stages.push({ target: PEAK_ITERATION_RATE, duration: `${holdSeconds}s` });
    }
    return {
      config: { ...base, executor: "ramping-arrival-rate", startRate: 0, stages },
      totalSeconds: rampSeconds + holdSeconds,
    };
  }
  return {
    config: {
      ...base,
      executor: "constant-arrival-rate",
      rate: PEAK_ITERATION_RATE,
      duration: `${SCENARIO_DURATION_SECONDS}s`,
    },
    totalSeconds: SCENARIO_DURATION_SECONDS,
  };
}

const { config: SCENARIO_CONFIG, totalSeconds: SCENARIO_TOTAL_SECONDS } = buildScenarioConfig();

// Envoy starts immediately; the chosen reverse proxy controller starts after Envoy's
// total runtime + the wait window so we don't synchronize request bursts.
const reverseProxyStartTime = `${SCENARIO_TOTAL_SECONDS + WAIT_BETWEEN_SCENARIOS}s`;
const reverseProxyScenarioName = `${PROXY_CONTROLLER}_simulation`;
const reverseProxyExec = PROXY_CONTROLLER === "kong" ? "kongScenario" : "nginxScenario";

export const options = {
  scenarios: {
    envoy_simulation: {
      ...SCENARIO_CONFIG,
      exec: "envoyScenario",
      startTime: "0s",
    },
    [reverseProxyScenarioName]: {
      ...SCENARIO_CONFIG,
      exec: reverseProxyExec,
      startTime: reverseProxyStartTime,
    },
  },
  thresholds: {
    // Per-controller latency thresholds aligned with SLO targets from
    // giantswarm/giantswarm#35147 (default: p95 < 500ms, p99 < 1000ms).
    // Scoped to status:200 — the authenticated path that actually reaches the
    // boutique — so the cheap gateway-side 401 reject path (which is tagged
    // expected_response:true by expectedStatuses) doesn't skew the percentiles.
    "http_req_duration{scenario:envoy_simulation,status:200}": [
      `p(95)<${SLO_P95_LATENCY_MS}`,
      `p(99)<${SLO_P99_LATENCY_MS}`,
    ],
    [`http_req_duration{scenario:${reverseProxyScenarioName},status:200}`]: [
      `p(95)<${SLO_P95_LATENCY_MS}`,
      `p(99)<${SLO_P99_LATENCY_MS}`,
    ],
    // Error rate: 401 responses are tagged expected (see expectedStatuses), so
    // http_req_failed only counts genuinely unexpected statuses.
    "http_req_failed{scenario:envoy_simulation}": [`rate<${SLO_ERROR_RATE}`],
    [`http_req_failed{scenario:${reverseProxyScenarioName}}`]: [`rate<${SLO_ERROR_RATE}`],
    // Auth assertions: valid->200, wrong->401, missing->401 must all hold.
    "checks{scenario:envoy_simulation}": [`rate>${SLO_CHECKS_RATE}`],
    [`checks{scenario:${reverseProxyScenarioName}}`]: [`rate>${SLO_CHECKS_RATE}`],
  },
};

// --- Basic-auth flow -------------------------------------------------------
// Hits the boutique homepage through the gateway with valid, wrong and missing
// credentials. checkHttp2=true for Envoy (HTTP/2 end-to-end); false for the
// reverse proxies (HTTP/1.1 to the upstream).

function basicAuthFlow(baseUrl, checkHttp2) {
  group("Valid credentials", function () {
    const res = http.get(`${baseUrl}/`, {
      headers: { Authorization: `Basic ${VALID_CREDENTIALS}` },
      responseCallback: expectedStatuses,
    });
    check(res, {
      "valid creds -> status 200": (r) => r.status === 200,
      "valid creds -> boutique served": (r) => r.body && r.body.includes("Online Boutique"),
      ...(checkHttp2 && { "valid creds -> protocol is HTTP/2": (r) => r.proto === "HTTP/2.0" }),
    });
  });

  group("Wrong credentials", function () {
    const res = http.get(`${baseUrl}/`, {
      headers: { Authorization: `Basic ${WRONG_CREDENTIALS}` },
      responseCallback: expectedStatuses,
    });
    check(res, {
      "wrong creds -> status 401": (r) => r.status === 401,
    });
  });

  group("Missing credentials", function () {
    const res = http.get(`${baseUrl}/`, {
      responseCallback: expectedStatuses,
    });
    check(res, {
      "missing creds -> status 401": (r) => r.status === 401,
    });
  });
}

// --- Scenario entry points ---

export function envoyScenario() {
  basicAuthFlow(pickEnvoyBaseUrl(), true);
}

export function nginxScenario() {
  basicAuthFlow(pickNginxBaseUrl(), false);
}

export function kongScenario() {
  basicAuthFlow(kongBaseUrl(), false);
}
