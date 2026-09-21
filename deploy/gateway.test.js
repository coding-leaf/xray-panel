import test, { describe, it, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";

import workerScript, {
  parseUpstreamOrigins,
  normalizeAndSanitizePath,
  evaluateCandidateResponse,
  createRoamingTimeoutSignal,
} from "./cloudflare-worker-sub-proxy.js";

import pagesScript from "./cloudflare-pages/_worker.js";

describe("Cloudflare Gateway Unit & Integration Tests", () => {
  let originalFetch;

  beforeEach(() => {
    originalFetch = globalThis.fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  describe("parseUpstreamOrigins", () => {
    it("should parse comma-separated origins and sanitize slashes and spaces", () => {
      const input = "  https://vps1.example.com/ ,  https://vps2.example.com///  ";
      const result = parseUpstreamOrigins(input);
      assert.deepEqual(result, [
        "https://vps1.example.com",
        "https://vps2.example.com",
      ]);
    });

    it("should parse newline-separated origins", () => {
      const input = "https://vps1.example.com\r\nhttps://vps2.example.com\nhttps://vps3.example.com";
      const result = parseUpstreamOrigins(input);
      assert.deepEqual(result, [
        "https://vps1.example.com",
        "https://vps2.example.com",
        "https://vps3.example.com",
      ]);
    });

    it("should fallback to defaultOrigin when rawConfig is empty or null", () => {
      const fallback = "https://default.example.com";
      assert.deepEqual(parseUpstreamOrigins("", fallback), ["https://default.example.com"]);
      assert.deepEqual(parseUpstreamOrigins(null, fallback), ["https://default.example.com"]);
      assert.deepEqual(parseUpstreamOrigins(undefined, fallback), ["https://default.example.com"]);
    });

    it("should filter out invalid URLs or unsupported protocols", () => {
      const input = "ftp://vps1.example.com, not-a-url, https://valid.example.com";
      const result = parseUpstreamOrigins(input);
      assert.deepEqual(result, ["https://valid.example.com"]);
    });

    it("should detect placeholder domain and return empty list", () => {
      const input = "https://panel.yourdomain.com";
      const result = parseUpstreamOrigins(input);
      assert.deepEqual(result, []);
    });
  });

  describe("Configuration & Placeholder Interception", () => {
    it("Worker: should return 500 when UPSTREAM_ORIGINS contains placeholder", async () => {
      const request = new Request("https://gateway.internal/api/portal/claim", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token: "abc" }),
      });
      const env = { UPSTREAM_ORIGINS: "https://panel.yourdomain.com" };

      const res = await workerScript.fetch(request, env, {});
      assert.equal(res.status, 500);
      const text = await res.text();
      assert.match(text, /500 Configuration Required/);
    });

    it("Pages: should return 500 when UPSTREAM_ORIGINS contains placeholder", async () => {
      const request = new Request("https://gateway.internal/api/portal/claim", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token: "abc" }),
      });
      const env = { UPSTREAM_ORIGINS: "https://panel.yourdomain.com" };

      const res = await pagesScript.fetch(request, env, {});
      assert.equal(res.status, 500);
      const text = await res.text();
      assert.match(text, /500 Configuration Required/);
    });
  });

  describe("Request Body Buffering & Multi-candidate Reuse", () => {
    it("should buffer POST body and successfully reuse it across multiple candidates", async () => {
      const requestBody = JSON.stringify({ code: "CLAIM-12345" });
      const calledHosts = [];
      const receivedBodies = [];

      globalThis.fetch = async (req) => {
        const url = new URL(req.url);
        calledHosts.push(url.host);
        const text = await req.text();
        receivedBodies.push(text);

        if (url.host === "vps1.example.com") {
          return new Response("Not found on node 1", { status: 404 });
        }
        if (url.host === "vps2.example.com") {
          return new Response(JSON.stringify({ success: true, node: "vps2" }), {
            status: 200,
            headers: {
              "Content-Type": "application/json",
              Server: "panel-nginx",
              "X-Powered-By": "Go",
            },
          });
        }
        return new Response("Unknown", { status: 500 });
      };

      const request = new Request("https://gateway.internal/api/portal/claim", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "CF-Connecting-IP": "203.0.113.195",
        },
        body: requestBody,
      });

      const env = {
        UPSTREAM_ORIGINS: "https://vps1.example.com, https://vps2.example.com",
      };

      const res = await workerScript.fetch(request, env, {});
      assert.equal(res.status, 200);
      const resJson = await res.json();
      assert.equal(resJson.success, true);
      assert.equal(resJson.node, "vps2");

      assert.deepEqual(calledHosts, ["vps1.example.com", "vps2.example.com"]);
      assert.deepEqual(receivedBodies, [requestBody, requestBody]);
      // Verify header sanitization
      assert.equal(res.headers.has("Server"), false);
      assert.equal(res.headers.has("X-Powered-By"), false);
      assert.equal(
        res.headers.get("Cache-Control"),
        "no-store, no-cache, must-revalidate, private"
      );
    });
  });

  describe("Fast Timeout Circuit Breaking (2500ms)", () => {
    it("should abort unresponsive node and continue to next node seamlessly", async () => {
      const calledHosts = [];

      globalThis.fetch = async (req, init) => {
        const url = new URL(req.url);
        calledHosts.push(url.host);

        if (url.host === "vps1.example.com") {
          // Simulate hanging node waiting for abort signal
          return new Promise((_, reject) => {
            const signal = init?.signal || req.signal;
            if (signal) {
              signal.addEventListener("abort", () => {
                reject(new DOMException("The operation was aborted.", "AbortError"));
              });
            }
          });
        }

        if (url.host === "vps2.example.com") {
          return new Response("Sub config content", {
            status: 200,
            headers: { "Content-Type": "text/plain" },
          });
        }

        return new Response("Error", { status: 500 });
      };

      const request = new Request("https://gateway.internal/sub?token=test-token", {
        method: "GET",
      });

      const env = {
        UPSTREAM_ORIGINS: "https://vps1.example.com, https://vps2.example.com",
      };

      const start = Date.now();
      const res = await workerScript.fetch(request, env, {});
      const elapsed = Date.now() - start;

      assert.equal(res.status, 200);
      const text = await res.text();
      assert.equal(text, "Sub config content");
      assert.deepEqual(calledHosts, ["vps1.example.com", "vps2.example.com"]);
      // Should have taken at least ~2500ms due to timeout on node 1
      assert.ok(elapsed >= 2400, `Expected elapsed >= 2400ms, got ${elapsed}ms`);
    });
  });

  describe("Hit 200 OK Stop & Header Sanitization", () => {
    it("should stop immediately on 200 OK and not probe remaining candidates", async () => {
      const calledHosts = [];

      globalThis.fetch = async (req) => {
        const url = new URL(req.url);
        calledHosts.push(url.host);
        return new Response("OK Node 1", {
          status: 200,
          headers: {
            "Content-Type": "text/plain",
            Server: "HiddenServer",
            "X-Powered-By": "CustomEngine",
          },
        });
      };

      const request = new Request("https://gateway.internal/sub/my-token", {
        method: "GET",
      });
      const env = {
        UPSTREAM_ORIGINS: "https://vps1.example.com, https://vps2.example.com, https://vps3.example.com",
      };

      const res = await workerScript.fetch(request, env, {});
      assert.equal(res.status, 200);
      assert.deepEqual(calledHosts, ["vps1.example.com"]);
      assert.equal(res.headers.has("Server"), false);
      assert.equal(res.headers.has("X-Powered-By"), false);
      assert.equal(
        res.headers.get("Cache-Control"),
        "no-store, no-cache, must-revalidate, private"
      );
    });
  });

  describe("400 / 403 / 404 Silent Progression", () => {
    it("should silently step to next candidate on 400, 403, or 404", async () => {
      const statuses = [404, 403, 400, 200];
      let callCount = 0;

      globalThis.fetch = async () => {
        const status = statuses[callCount++];
        return new Response(`Node ${callCount} response`, { status });
      };

      const request = new Request("https://gateway.internal/sub?token=test", {
        method: "GET",
      });
      const env = {
        UPSTREAM_ORIGINS:
          "https://node1.com, https://node2.com, https://node3.com, https://node4.com",
      };

      const res = await workerScript.fetch(request, env, {});
      assert.equal(res.status, 200);
      assert.equal(callCount, 4);
    });
  });

  describe("All Exhausted Neutral Fallback", () => {
    it("/api/portal/claim: should return neutral JSON 404 when all nodes return 404", async () => {
      globalThis.fetch = async () => {
        return new Response("Not found", { status: 404 });
      };

      const request = new Request("https://gateway.internal/api/portal/claim", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token: "wrong" }),
      });
      const env = {
        UPSTREAM_ORIGINS: "https://vps1.com, https://vps2.com",
      };

      const res = await workerScript.fetch(request, env, {});
      assert.equal(res.status, 404);
      assert.equal(
        res.headers.get("Content-Type"),
        "application/json; charset=utf-8"
      );
      assert.equal(
        res.headers.get("Cache-Control"),
        "no-store, no-cache, must-revalidate, private"
      );
      const data = await res.json();
      assert.deepEqual(data, { error: "凭据无效、已过期或未配置" });
    });

    it("/sub: should return neutral text 404 when all nodes return 404", async () => {
      globalThis.fetch = async () => {
        return new Response("Not found", { status: 404 });
      };

      const request = new Request("https://gateway.internal/sub?token=wrong", {
        method: "GET",
      });
      const env = {
        UPSTREAM_ORIGINS: "https://vps1.com, https://vps2.com",
      };

      const res = await workerScript.fetch(request, env, {});
      assert.equal(res.status, 404);
      assert.equal(
        res.headers.get("Content-Type"),
        "text/plain; charset=utf-8"
      );
      assert.equal(
        res.headers.get("Cache-Control"),
        "no-store, no-cache, must-revalidate, private"
      );
      const text = await res.text();
      assert.equal(text, "404 Not Found");
    });

    it("Worker: should return 502 Bad Gateway when all nodes timeout or 5xx", async () => {
      globalThis.fetch = async () => {
        return new Response("Internal Error", { status: 500 });
      };

      const request = new Request("https://gateway.internal/sub?token=any", {
        method: "GET",
      });
      const env = {
        UPSTREAM_ORIGINS: "https://vps1.com, https://vps2.com",
      };

      const res = await workerScript.fetch(request, env, {});
      assert.equal(res.status, 502);
      const text = await res.text();
      assert.equal(text, "Gateway upstream connection timeout or reset.");
    });

    it("Pages: should return 502 Bad Gateway when all nodes timeout or 5xx", async () => {
      globalThis.fetch = async () => {
        return new Response("Internal Error", { status: 503 });
      };

      const request = new Request("https://gateway.internal/sub?token=any", {
        method: "GET",
      });
      const env = {
        UPSTREAM_ORIGINS: "https://vps1.com, https://vps2.com",
      };

      const res = await pagesScript.fetch(request, env, {});
      assert.equal(res.status, 502);
      const text = await res.text();
      assert.equal(text, "Service Gateway Temporarily Unavailable");
    });
  });

  describe("Pages Specific Features", () => {
    it("should passthrough / and /portal to env.ASSETS.fetch", async () => {
      let assetsFetched = false;
      const env = {
        UPSTREAM_ORIGINS: "https://vps1.com",
        ASSETS: {
          fetch: async (req) => {
            assetsFetched = true;
            return new Response("Static HTML", { status: 200 });
          },
        },
      };

      const reqRoot = new Request("https://pages.internal/");
      const resRoot = await pagesScript.fetch(reqRoot, env, {});
      assert.equal(resRoot.status, 200);
      assert.equal(assetsFetched, true);

      assetsFetched = false;
      const reqPortal = new Request("https://pages.internal/portal");
      const resPortal = await pagesScript.fetch(reqPortal, env, {});
      assert.equal(resPortal.status, 200);
      assert.equal(assetsFetched, true);
    });

    it("Pages: should buffer POST body and successfully reuse it across candidates", async () => {
      const requestBody = JSON.stringify({ claim_code: "PAGES-999" });
      const calledHosts = [];

      globalThis.fetch = async (req) => {
        const url = new URL(req.url);
        calledHosts.push(url.host);
        if (url.host === "pages-node1.com") {
          return new Response("Miss", { status: 404 });
        }
        return new Response(JSON.stringify({ ok: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      };

      const request = new Request("https://pages.internal/api/portal/claim", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: requestBody,
      });

      const env = {
        UPSTREAM_ORIGINS: "https://pages-node1.com, https://pages-node2.com",
      };

      const res = await pagesScript.fetch(request, env, {});
      assert.equal(res.status, 200);
      assert.deepEqual(calledHosts, ["pages-node1.com", "pages-node2.com"]);
    });

    it("should return 200 OK innocent page for bot UA", async () => {
      const request = new Request("https://pages.internal/portal", {
        headers: { "User-Agent": "Baiduspider+(+http://www.baidu.com/search/spider.htm)" },
      });
      const env = { UPSTREAM_ORIGINS: "https://vps1.com" };

      const res = await pagesScript.fetch(request, env, {});
      assert.equal(res.status, 200);
      const text = await res.text();
      assert.match(text, /Service Gateway Online/);
    });
  });

  describe("Backward Compatibility & Security Baseline", () => {
    it("should fallback cleanly to single UPSTREAM_ORIGIN", async () => {
      globalThis.fetch = async (req) => {
        return new Response("Single node success", { status: 200 });
      };

      const request = new Request("https://gateway.internal/sub?token=compat", {
        method: "GET",
      });
      const env = {
        UPSTREAM_ORIGIN: "https://single-node.example.com",
      };

      const res = await workerScript.fetch(request, env, {});
      assert.equal(res.status, 200);
      const text = await res.text();
      assert.equal(text, "Single node success");
    });

    it("should block path traversal attempts with 400 Bad Request", async () => {
      const request = new Request(
        "https://gateway.internal/api/portal/%252e%252e/admin"
      );
      const env = { UPSTREAM_ORIGINS: "https://vps1.com" };

      const res = await workerScript.fetch(request, env, {});
      assert.equal(res.status, 400);
    });

    it("normalizeAndSanitizePath should sanitize double encoding and detect traversal", () => {
      assert.equal(normalizeAndSanitizePath("/portal"), "/portal");
      assert.equal(normalizeAndSanitizePath("/sub/my-token"), "/sub/my-token");
      assert.equal(normalizeAndSanitizePath("/api/portal/../secret"), null);
      assert.equal(normalizeAndSanitizePath("/api/portal/%2e%2e/secret"), null);
      assert.equal(normalizeAndSanitizePath("/api/portal/%252e%252e/secret"), null);
      assert.equal(normalizeAndSanitizePath("/api/portal/test\\admin"), null);
      assert.equal(normalizeAndSanitizePath("/api/portal/test%00admin"), null);
    });

    it("evaluateCandidateResponse should correctly classify actions and categories", () => {
      assert.deepEqual(evaluateCandidateResponse(200, false), {
        action: "HIT",
        category: "SUCCESS",
      });
      assert.deepEqual(evaluateCandidateResponse(404, false), {
        action: "NEXT",
        category: "CLIENT_MISS",
      });
      assert.deepEqual(evaluateCandidateResponse(400, false), {
        action: "NEXT",
        category: "CLIENT_MISS",
      });
      assert.deepEqual(evaluateCandidateResponse(500, false), {
        action: "NEXT",
        category: "SERVER_ERROR",
      });
      assert.deepEqual(evaluateCandidateResponse(0, true), {
        action: "NEXT",
        category: "NETWORK_ERROR",
      });
    });
  });
});
