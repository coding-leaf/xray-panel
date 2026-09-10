/**
 * ==============================================================================
 * Cloudflare Pages: 免翻墙中立门户网关与反探测代理 (v3.1 Pages 生产加固版)
 * ==============================================================================
 * 
 * 核心设计：
 * 1. 根路径 (/)：展示纯良中立的“鹈鹕骑行俱乐部”主页，杜绝暴露任何代理/后台特征；
 * 2. 提取门户 (/portal)：代理反向穿透至真实的 VPS 面板安全提取页面；
 * 3. 凭证兑换 (/api/portal/claim)：极速安全转发，强制 Cache-Control: no-store，绝不缓存凭据；
 * 4. 订阅分发 (/sub/*)：透传节点订阅，自动注入 X-Forwarded-Proto 与 X-Forwarded-Host；
 * 5. 爬虫欺骗：腾讯/微信扫描爬虫命中时返回 200 OK，维护域名极佳的信誉评级。
 */

// 默认兜底后端 VPS 面板地址（通用开源占位符，请在 Cloudflare Pages 的 Settings -> Environment Variables 中配置 UPSTREAM_ORIGIN）
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
    const upstreamBase = env?.UPSTREAM_ORIGIN || DEFAULT_UPSTREAM_ORIGIN;

    // 检查是否配置了真实源站环境变量
    if (upstreamBase.includes("yourdomain.com")) {
      return new Response(
        "500 Configuration Required: Please configure UPSTREAM_ORIGIN in Cloudflare Pages Settings -> Environment variables, then trigger a new deployment.",
        { status: 500, headers: { "Content-Type": "text/plain; charset=utf-8" } }
      );
    }

    // 1. 爬虫伪装防御：检测到腾讯/扫描爬虫时返回 200 OK 中立健康页
    const isBot = BOT_UA_PATTERNS.some((pattern) => pattern.test(userAgent));
    if (isBot) {
      return makeInnocentBotResponse();
    }

    // 2. 静态页面直通：
    // - 根路径 (/) -> 鹈鹕骑行俱乐部伪装页
    // - 提取门户 (/portal) -> 独立纯净提取单页 (Cloudflare Edge 极速直达，零后台代码、零闪烁)
    if (url.pathname === "/" || url.pathname === "" || url.pathname === "/index.html") {
      if (env?.ASSETS) {
        return env.ASSETS.fetch(request);
      }
    }

    if (url.pathname === "/portal" || url.pathname === "/portal/" || url.pathname === "/portal.html") {
      if (env?.ASSETS) {
        const portalUrl = new URL("/portal.html", request.url);
        return env.ASSETS.fetch(new Request(portalUrl, request));
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

    // 5. 构建发往真实 VPS 源站的上游请求（严格使用净化后的 normalizedPath）
    const upstreamUrl = new URL(normalizedPath + url.search, upstreamBase);
    const newHeaders = new Headers(request.headers);

    // 6. 真实 IP、域名与协议安全注入
    const clientIP = request.headers.get("CF-Connecting-IP") || "127.0.0.1";
    newHeaders.set("X-Real-IP", clientIP);
    newHeaders.set("X-Forwarded-For", clientIP);
    newHeaders.set("X-Forwarded-Proto", "https"); // 告知后端当前经由 HTTPS 访问
    newHeaders.set("X-Forwarded-Host", url.host); // 告知后端 Pages 的外部访问域名 (如 xxx.pages.dev)
    newHeaders.set("Host", upstreamUrl.host);

    // 7. 遵守 WHATWG Fetch 规范：GET / HEAD 请求强制置空 body
    const isBodyAllowed = !["GET", "HEAD"].includes(request.method.toUpperCase());
    const modifiedRequest = new Request(upstreamUrl.toString(), {
      method: request.method,
      headers: newHeaders,
      body: isBodyAllowed ? request.body : null,
      redirect: "follow",
    });

    // 8. 代理回源执行与敏感标头净化
    try {
      const response = await fetch(modifiedRequest);
      const resHeaders = new Headers(response.headers);

      // 剥离暴露源站技术的指纹标头
      resHeaders.delete("Server");
      resHeaders.delete("X-Powered-By");

      // 关键安全项：对凭据兑换接口与聚合订阅接口强制注入 no-store，严禁 Cloudflare 边缘缓存敏感凭据
      if (normalizedPath.startsWith("/api/portal/") || normalizedPath.startsWith("/sub")) {
        resHeaders.set("Cache-Control", "no-store, no-cache, must-revalidate, private");
        resHeaders.set("Pragma", "no-cache");
      }

      return new Response(response.body, {
        status: response.status,
        statusText: response.statusText,
        headers: resHeaders,
      });
    } catch (err) {
      return new Response("Service Gateway Temporarily Unavailable", {
        status: 502,
        headers: { "Content-Type": "text/plain; charset=utf-8" },
      });
    }
  },
};
