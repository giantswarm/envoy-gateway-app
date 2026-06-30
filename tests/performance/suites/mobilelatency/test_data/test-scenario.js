import http from "k6/http";
import { check } from "k6";

// Mobile latency-distribution replay. Ported from
// cni-gateway-api-use-cases-k6/k6/static-use-cases/mobile-latency.js. It
// reproduces a real production traffic mix (see MOBILE.md — a 17.6M-calls/hour
// Kong sample) as five latency bands, each its own constant-arrival-rate
// scenario driven at the band's measured RPS. The backend (go-httpbin) is told
// to sleep per request via /delay/{seconds}, so the gateway has to juggle fast
// and very slow upstreams concurrently — the point of the test.
//
// Mirrors the "basic" suite's Envoy-vs-reverse-proxy layout: the five envoy
// bands run first, then the five reverse-proxy bands after a wait, so the two
// don't synchronize. PROXY_CONTROLLER selects nginx or kong.

const BASE_DOMAIN = __ENV.BASE_DOMAIN;
const ENDPOINTS = parseInt(__ENV.ENDPOINTS || "10", 10);
const PROXY_CONTROLLER = (__ENV.PROXY_CONTROLLER || "nginx").toLowerCase();
if (PROXY_CONTROLLER !== "nginx" && PROXY_CONTROLLER !== "kong") {
  throw new Error(`PROXY_CONTROLLER must be 'nginx' or 'kong' (got: '${PROXY_CONTROLLER}')`);
}

// Per-simulation duration (k6 duration string), the wait inserted before the
// reverse-proxy simulation, and the factor that scales the production RPS down
// to a test cluster.
const RUN_DURATION = __ENV.RUN_DURATION || "20m";
const WAIT_BETWEEN_SCENARIOS = parseInt(__ENV.WAIT_BETWEEN_SCENARIOS || "300", 10);
const REDUCTION_RPS_FACTOR = Math.max(1, parseInt(__ENV.REDUCTION_RPS_FACTOR || "10", 10));
const GRACEFUL_STOP = __ENV.GRACEFUL_STOP || "30s";

const SLO_ERROR_RATE  = __ENV.SLO_ERROR_RATE  || "0.001"; // 0.1%
const SLO_CHECKS_RATE = __ENV.SLO_CHECKS_RATE || "0.99";

// httpbin is overlaid on the /delay path of the existing boutique hosts (so DNS
// + TLS are reused). Envoy has one endpoint per loadtesting-{i} namespace, so
// each request picks one at random to spread load across all of them, the way
// the other suites' envoy scenarios do. nginx/kong each expose a single host.
function envoyBaseUrl() {
  const n = Math.floor(Math.random() * ENDPOINTS);
  return `https://onlineboutique.loadtesting-${n}.${BASE_DOMAIN}`;
}
function nginxBaseUrl() { return `https://nginx-onlineboutique-0.loadtesting.${BASE_DOMAIN}`; }
function kongBaseUrl()  { return `https://kong-onlineboutique.loadtesting.${BASE_DOMAIN}`; }
function proxyBaseUrl() { return PROXY_CONTROLLER === "kong" ? kongBaseUrl() : nginxBaseUrl(); }

// Latency bands (requests/second + injected delay in seconds), from
// mobile-latency.js / MOBILE.md. go-httpbin is deployed with -max-duration=60s
// so the 12s "over_10s" band is served faithfully (the default cap is 10s).
const BANDS = [
  { key: "fast_under_10ms", rps: 1666, delay: 0.005 },
  { key: "under_100ms",     rps: 777,  delay: 0.05 },
  { key: "under_200ms",     rps: 2194, delay: 0.2 },
  { key: "under_10s",       rps: 222,  delay: 5 },
  { key: "over_10s",        rps: 27,   delay: 12 },
];

function parseDurationSeconds(d) {
  const m = /^(\d+)(ms|s|m|h)?$/.exec(String(d).trim());
  if (!m) return 1200;
  const value = parseInt(m[1], 10);
  switch (m[2]) {
    case "h": return value * 3600;
    case "m": return value * 60;
    case "ms": return Math.ceil(value / 1000);
    default: return value; // "s" or bare number
  }
}

// makeScenario mirrors the upstream VU sizing: for constant-arrival-rate the
// concurrency needed is ~ rate * iteration_duration, with headroom so arrivals
// aren't dropped. RPS is scaled down by REDUCTION_RPS_FACTOR.
function makeScenario(exec, rps, delaySeconds, startTime, proxy) {
  const rate = Math.max(1, Math.ceil(rps / REDUCTION_RPS_FACTOR));
  const iterationSeconds = Math.max(0.5, delaySeconds);
  const concurrency = Math.ceil(rate * iterationSeconds);
  return {
    executor: "constant-arrival-rate",
    rate,
    timeUnit: "1s",
    duration: RUN_DURATION,
    startTime,
    preAllocatedVUs: Math.max(20, Math.ceil(concurrency * 1.25)),
    maxVUs: Math.max(50, Math.ceil(concurrency * 8.0)),
    gracefulStop: GRACEFUL_STOP,
    exec,
    tags: { proxy },
  };
}

// Envoy runs first; the reverse proxy starts after Envoy's full duration plus
// the wait window so request bursts don't overlap.
const proxyStartTime = `${parseDurationSeconds(RUN_DURATION) + WAIT_BETWEEN_SCENARIOS}s`;

const scenarios = {};
const thresholds = {};
for (const b of BANDS) {
  const envoyExec = `envoy_${b.key}`;
  const proxyExec = `proxy_${b.key}`;
  scenarios[envoyExec] = makeScenario(envoyExec, b.rps, b.delay, "0s", "envoy");
  scenarios[proxyExec] = makeScenario(proxyExec, b.rps, b.delay, proxyStartTime, PROXY_CONTROLLER);
  for (const s of [envoyExec, proxyExec]) {
    // Every completed request must be a 200; the injected delay means absolute
    // latency thresholds aren't meaningful per band, so we gate on success rate
    // and error rate (and that the band actually ran).
    thresholds[`checks{scenario:${s}}`] = [`rate>${SLO_CHECKS_RATE}`];
    thresholds[`http_req_failed{scenario:${s}}`] = [`rate<${SLO_ERROR_RATE}`];
    thresholds[`http_reqs{scenario:${s}}`] = ["count>0"];
  }
}

export const options = { scenarios, thresholds };

// --- request ---------------------------------------------------------------

function hitDelay(baseUrl, seconds) {
  const res = http.get(`${baseUrl}/delay/${seconds}`);
  check(res, {
    "status is 200": (r) => r.status === 200,
  });
}

// --- scenario entry points (one per band, per side) ------------------------

export function envoy_fast_under_10ms() { hitDelay(envoyBaseUrl(), 0.005); }
export function envoy_under_100ms()     { hitDelay(envoyBaseUrl(), 0.05); }
export function envoy_under_200ms()     { hitDelay(envoyBaseUrl(), 0.2); }
export function envoy_under_10s()       { hitDelay(envoyBaseUrl(), 5); }
export function envoy_over_10s()        { hitDelay(envoyBaseUrl(), 12); }

export function proxy_fast_under_10ms() { hitDelay(proxyBaseUrl(), 0.005); }
export function proxy_under_100ms()     { hitDelay(proxyBaseUrl(), 0.05); }
export function proxy_under_200ms()     { hitDelay(proxyBaseUrl(), 0.2); }
export function proxy_under_10s()       { hitDelay(proxyBaseUrl(), 5); }
export function proxy_over_10s()        { hitDelay(proxyBaseUrl(), 12); }
