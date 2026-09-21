/**
 * ==============================================================================
 * Cloudflare Pages: 免翻墙中立门户网关与多源站弹性漫游反探测代理 (v4.0 Pages 生产版)
 * ==============================================================================
 * 
 * 核心设计：
 * 1. 根路径 (/)：展示中立公开静态主页，杜绝暴露任何代理/后台特征；
 * 2. 提取门户 (/portal)：极速直通静态资产单页，经由 API 反向穿透至各 VPS 面板；
 * 3. 多源站顺序漫游：支持 UPSTREAM_ORIGINS 逗号/换行分隔多节点，单机 2.5s 超时极速熔断并自动寻呼；
 * 4. 请求体跨机复用：非 GET/HEAD 请求通过 ArrayBuffer 预读缓冲，杜绝 body already consumed；
 * 5. 凭证兑换 (/api/portal/claim)：极速安全转发，强制 Cache-Control: no-store，绝不缓存凭据；
 * 6. 订阅分发 (/sub/*)：透传节点订阅，自动注入 X-Forwarded-Proto 与 X-Forwarded-Host；
 * 7. 爬虫欺骗：腾讯/微信扫描爬虫命中时返回 200 OK，维护域名极佳的信誉评级；
 * 8. 源站指纹剥离：隐匿 Server 与 X-Powered-By，全节点穷尽后对外输出统一中立安全兜底。
 */

// 默认兜底后端 VPS 面板地址（通用开源占位符，请在 Cloudflare Pages 的 Settings -> Environment Variables 中配置 UPSTREAM_ORIGINS）
const DEFAULT_UPSTREAM_ORIGIN = "https://panel.yourdomain.com";

// 审查爬虫与自动化扫描特征正则（精准识别，绝不误杀真实普通微信/移动端用户）
const BOT_UA_PATTERNS = [
  /MicroMessengerBot/i, // 微信官方网页抓取与安全扫描爬虫
  /mpcrawler/i,         // 微信公众号爬虫
  /TencentTraveler/i,   // 腾讯分析扫描爬虫
  /QQDownload/i,        // QQ 安全检测爬虫
  /Baiduspider/i,
  /YisouSpider/i,
  /Bytespider/i,
  /Sogou/i,
  /Googlebot/i,
  /bingbot/i,
  /SemrushBot/i,
  /AhrefsBot/i,
  /HeadlessChrome/i,    // 无头自动化抓取浏览器
  /sqlmap/i,
  /nikto/i,
  /nmap/i,
  /curl\//i,
  /python-requests/i,
];

// 严格白名单集合（全段安全比对）
const EXACT_ALLOWED_PATHS = new Set([
  "/",
  "/index.html",
  "/portal",
  "/favicon.svg",
  "/sub", // 兼容 /sub?token=xxx 参数模式
]);

const PREFIX_ALLOWED_PATHS = [
  "/portal/",
  "/api/portal/",
  "/assets/",
  "/sub/", // 兼容 /sub/:token 路径模式
];

/**
 * 解析并清洗上游多源站配置
 * @param {string|null|undefined} rawConfig - 原始配置字符串（支持逗号或换行分隔）
 * @param {string} [defaultOrigin] - 默认兜底源站
 * @returns {string[]} 有效的标准化候选源站列表
 */
function parseUpstreamOrigins(rawConfig, defaultOrigin = DEFAULT_UPSTREAM_ORIGIN) {
  const config =
    rawConfig !== undefined && rawConfig !== null && String(rawConfig).trim() !== ""
      ? String(rawConfig)
      : defaultOrigin;

  if (!config) return [];

  // 占位符拦截：若配置包含默认模板占位符，直接判定未配置
  if (config.includes("yourdomain.com")) {
    return [];
  }

  const parts = config.split(/[\r\n,]+/);
  const origins = [];

  for (let part of parts) {
    part = part.trim();
    if (!part) continue;
    if (part.includes("yourdomain.com")) {
      return [];
    }
    // 去除 URL 尾部的末尾斜杠
    part = part.replace(/\/+$/, "");
    try {
      const parsed = new URL(part);
      if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
        continue;
      }
      origins.push(part);
    } catch {
      continue;
    }
  }

  return origins;
}

/**
 * 严格路径规范化：执行多重 URL 解码直至收敛，消除所有穿越向量
 */
function normalizeAndSanitizePath(rawPath) {
  let path = rawPath;

  // 1. Fixed-point 多重解码：最多循环 3 轮，防御 %252e%252e 等双重 URL 编码攻击
  for (let i = 0; i < 3; i++) {
    try {
      const decoded = decodeURIComponent(path);
      if (decoded === path) break;
      path = decoded;
    } catch {
      return null; // 非法畸形编码直接阻断
    }
  }

  // 2. 严格特征防御：解码后若仍然包含路径穿越相对段 '..'、反斜杠 '\'、空字节 '\0' 或未解析的 '%'，直接拒绝
  if (path.includes("..") || path.includes("\\") || path.includes("\0") || path.includes("%")) {
    return null;
  }

  // 3. 利用 WHATWG URL 解析消除规范化点段
  try {
    const dummyUrl = new URL(path, "https://gateway.internal");
    return dummyUrl.pathname;
  } catch {
    return null;
  }
}

/**
 * 漫游超时信号发生器（2500ms 极速超时熔断）
 * 采用显式 AbortController + setTimeout 配合 cleanup 彻底杜绝定时器泄露
 */
function createRoamingTimeoutSignal(timeoutMs = 2500) {
  const controller = new AbortController();
  const timer = setTimeout(() => {
    controller.abort(new DOMException("The operation was aborted due to timeout", "TimeoutError"));
  }, timeoutMs);
  return {
    signal: controller.signal,
    cleanup: () => clearTimeout(timer),
  };
}

/**
 * 候选源站响应评估状态机
 */
function evaluateCandidateResponse(status, isAbortedOrNetworkError) {
  if (isAbortedOrNetworkError) {
    return { action: "NEXT", category: "NETWORK_ERROR" };
  }
  if (status >= 200 && status < 300) {
    return { action: "HIT", category: "SUCCESS" };
  }
  if (status === 400 || status === 403 || status === 404) {
    return { action: "NEXT", category: "CLIENT_MISS" };
  }
  if (status >= 500) {
    return { action: "NEXT", category: "SERVER_ERROR" };
  }
  return { action: "NEXT", category: "OTHER" };
}

/**
 * 爬虫伪装响应：返回 200 OK 的合法中立“服务运行中”页面，维持域名高信誉评级
 */
function makeInnocentBotResponse() {
  const html = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Service Gateway Online</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; background: #f8fafc; color: #334155; }
    .card { text-align: center; padding: 2rem; border-radius: 12px; background: white; box-shadow: 0 1px 3px rgba(0,0,0,0.1); border: 1px solid #e2e8f0; max-width: 360px; width: 90%; }
    .status { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 500; color: #16a34a; background: #f0fdf4; padding: 4px 10px; border-radius: 9999px; margin-bottom: 12px; }
    .dot { width: 8px; height: 8px; border-radius: 50%; background: #16a34a; }
    h2 { font-size: 18px; margin: 0 0 8px 0; color: #0f172a; }
    p { font-size: 13px; margin: 0; color: #64748b; line-height: 1.5; }
  </style>
</head>
<body>
  <div class="card">
    <div class="status"><span class="dot"></span>System Operational</div>
    <h2>Data Exchange Gateway</h2>
    <p>The gateway service is active and running normally. Verification protocols are active.</p>
  </div>
</body>
</html>`;
  return new Response(html, {
    status: 200,
    headers: {
      "Content-Type": "text/html; charset=utf-8",
      "Cache-Control": "public, max-age=3600",
    },
  });
}

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    const userAgent = request.headers.get("User-Agent") || "";

    // 1. 爬虫伪装防御：检测到腾讯/扫描爬虫时返回 200 OK 中立健康页
    const isBot = BOT_UA_PATTERNS.some((pattern) => pattern.test(userAgent));
    if (isBot) {
      return makeInnocentBotResponse();
    }

    // 2. 静态页面直通：
    // - 根路径 (/) -> 中立公开静态页面
    // - 提取门户 (/portal) -> 独立纯净提取单页 (Cloudflare Edge 极速直达，零后台代码、零闪烁)
    if (url.pathname === "/" || url.pathname === "" || url.pathname === "/index.html") {
      if (env?.ASSETS) {
        return env.ASSETS.fetch(request);
      }
    }

    if (
      url.pathname === "/portal" ||
      url.pathname === "/portal/" ||
      url.pathname === "/portal.html" ||
      url.pathname === "/portal/index.html"
    ) {
      if (env?.ASSETS) {
        return env.ASSETS.fetch(request);
      }
    }

    // 3. 路径多重解码与穿越攻击净化 (Anti-Double-Encoding & Path-Traversal)
    const normalizedPath = normalizeAndSanitizePath(url.pathname);
    if (!normalizedPath) {
      return new Response("400 Bad Request", { status: 400 });
    }

    // 4. 严格全段白名单校验
    const isAllowed =
      EXACT_ALLOWED_PATHS.has(normalizedPath) ||
      PREFIX_ALLOWED_PATHS.some((prefix) => normalizedPath.startsWith(prefix));

    if (!isAllowed) {
      return new Response("404 Not Found", { status: 404 });
    }

    // 5. 多源站配置解析与占位符拦截
    const rawOrigins = env?.UPSTREAM_ORIGINS || env?.UPSTREAM_ORIGIN;
    const candidates = parseUpstreamOrigins(rawOrigins, DEFAULT_UPSTREAM_ORIGIN);

    if (!candidates || candidates.length === 0) {
      return new Response(
        "500 Configuration Required: Please configure UPSTREAM_ORIGINS in Cloudflare Pages Settings -> Environment variables, then trigger a new deployment.",
        { status: 500, headers: { "Content-Type": "text/plain; charset=utf-8" } }
      );
    }

    // 6. 请求体预读缓存（ArrayBuffer 支持多次安全复用，规避 body already consumed 异常）
    const isBodyAllowed = !["GET", "HEAD"].includes(request.method.toUpperCase());
    let cachedBody = null;
    if (isBodyAllowed) {
      cachedBody = await request.arrayBuffer();
    }

    const clientIP = request.headers.get("CF-Connecting-IP") || "127.0.0.1";

    let hasClientMiss = false;
    let hasServerError = false;
    let hasNetworkError = false;

    // 7. 顺序漫游寻呼状态机 (Sequential Roaming Loop)
    for (const upstreamBase of candidates) {
      const upstreamUrl = new URL(normalizedPath + url.search, upstreamBase);
      const newHeaders = new Headers(request.headers);

      // 真实 IP、域名与协议安全注入
      newHeaders.set("CF-Connecting-IP", clientIP);
      newHeaders.set("X-Real-IP", clientIP);
      newHeaders.set("X-Forwarded-For", clientIP);
      newHeaders.set("X-Forwarded-Proto", "https");
      newHeaders.set("X-Forwarded-Host", url.host);
      newHeaders.set("Host", upstreamUrl.host);

      const modifiedRequest = new Request(upstreamUrl.toString(), {
        method: request.method,
        headers: newHeaders,
        body: isBodyAllowed ? cachedBody : null,
        redirect: "follow",
      });

      const { signal, cleanup } = createRoamingTimeoutSignal(2500);

      try {
        let response;
        try {
          response = await fetch(modifiedRequest, { signal });
        } finally {
          cleanup();
        }

        const evaluation = evaluateCandidateResponse(response.status, false);

        // 命中即止 (Hit Stop)
        if (evaluation.action === "HIT") {
          const resHeaders = new Headers(response.headers);

          // 剥离暴露源站技术的指纹标头
          resHeaders.delete("Server");
          resHeaders.delete("X-Powered-By");

          // 注入基础 Web 安全防护标头
          resHeaders.set("X-Frame-Options", "DENY");
          resHeaders.set("X-Content-Type-Options", "nosniff");
          resHeaders.set("Referrer-Policy", "no-referrer");

          // 关键安全项：敏感数据路由禁止缓存
          if (normalizedPath.startsWith("/api/portal") || normalizedPath.startsWith("/sub")) {
            resHeaders.set("Cache-Control", "no-store, no-cache, must-revalidate, private");
            resHeaders.set("Pragma", "no-cache");
            resHeaders.set("Expires", "0");
          } else if (normalizedPath === "/portal" || normalizedPath.startsWith("/portal/")) {
            resHeaders.set("Cache-Control", "no-cache");
          }

          return new Response(response.body, {
            status: response.status,
            statusText: response.statusText,
            headers: resHeaders,
          });
        }

        // 状态机记录
        if (evaluation.category === "CLIENT_MISS") {
          hasClientMiss = true;
        } else if (evaluation.category === "SERVER_ERROR") {
          hasServerError = true;
        }
      } catch (err) {
        cleanup();
        hasNetworkError = true;
      }
    }

    // 8. 漫游全穷尽中立兜底处理 (Neutral Fallback)
    if (hasClientMiss) {
      if (normalizedPath.startsWith("/api/portal/claim")) {
        return new Response(JSON.stringify({ error: "凭据无效、已过期或未配置" }), {
          status: 404,
          headers: {
            "Content-Type": "application/json; charset=utf-8",
            "Cache-Control": "no-store, no-cache, must-revalidate, private",
          },
        });
      }
      return new Response("404 Not Found", {
        status: 404,
        headers: {
          "Content-Type": "text/plain; charset=utf-8",
          "Cache-Control": "no-store, no-cache, must-revalidate, private",
        },
      });
    }

    // 全部节点超时、阻断或 5xx 故障
    return new Response("Service Gateway Temporarily Unavailable", {
      status: 502,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  },
};

export {
  DEFAULT_UPSTREAM_ORIGIN,
  BOT_UA_PATTERNS,
  EXACT_ALLOWED_PATHS,
  PREFIX_ALLOWED_PATHS,
  parseUpstreamOrigins,
  normalizeAndSanitizePath,
  createRoamingTimeoutSignal,
  evaluateCandidateResponse,
  makeInnocentBotResponse,
};

