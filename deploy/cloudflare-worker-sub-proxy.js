/**
 * ==============================================================================
 * Cloudflare Worker: 免翻墙中立门户网关与反探测代理 (v3.0 生产工业级加固版)
 * ==============================================================================
 * 
 * 安全审计加固项：
 * 1. 终结多重编码绕过：实现 Fixed-point 收敛解码，严密拦截 %252e 等双重/多重 URL 编码路径穿越；
 * 2. 敏感数据防缓存注入：对 /api/portal/ 与 /sub 接口强制注入 Cache-Control: no-store，杜绝边缘串号与凭据泄露；
 * 3. 补齐 HTTPS 协议标头：显式注入 X-Forwarded-Proto: https，确保后端生成标准合法订阅链接；
 * 4. 完美兼容 /sub 接口：同时放行 /sub/:token 路径模式与 /sub?token=... 参数查询模式；
 * 5. 环境变量解耦：优先读取 env.UPSTREAM_ORIGIN，支持在 Cloudflare 控制台热更新与密钥保护；
 * 6. 爬虫欺骗高存活策略：检测到审查爬虫时返回 200 OK 中立“健康运行”空白页，杜绝被腾讯判定为异常站点弹窗报红。
 */

// 默认兜底后端 VPS 面板地址（生产环境推荐在 Cloudflare Worker 的 Settings -> Variables 中配置 UPSTREAM_ORIGIN）
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
      return null;
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

    // 1. 爬虫伪装防御：返回 200 OK 中立正常页，杜绝被腾讯标记为不可访问站点
    const isBot = BOT_UA_PATTERNS.some((pattern) => pattern.test(userAgent));
    if (isBot) {
      return makeInnocentBotResponse();
    }

    // 2. 根路径保护：访问 / 强制 302 跳转至 https://.../portal，杜绝暴露后台 SPA 登录页
    if (url.pathname === "/" || url.pathname === "") {
      return Response.redirect(`https://${url.host}/portal`, 302);
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

    // 6. 真实 IP 与协议安全注入
    const clientIP = request.headers.get("CF-Connecting-IP") || "127.0.0.1";
    newHeaders.set("X-Real-IP", clientIP);
    newHeaders.set("X-Forwarded-For", clientIP);
    newHeaders.set("X-Forwarded-Proto", "https"); // 告知后端当前经由 HTTPS 访问，生成正确的订阅 URL 前缀
    newHeaders.set("X-Forwarded-Host", url.host); // 告知后端 Worker 的外部访问域名
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

      // 注入基础 Web 安全防护标头 (防点击劫持、防 MIME 嗅探、防 Referer 泄漏)
      resHeaders.set("X-Frame-Options", "DENY");
      resHeaders.set("X-Content-Type-Options", "nosniff");
      resHeaders.set("Referrer-Policy", "no-referrer");

      // 关键安全防线：对包含临时凭据与节点明文的路由强制禁止任何边缘或共享缓存
      if (normalizedPath.startsWith("/api/portal") || normalizedPath.startsWith("/sub")) {
        resHeaders.set("Cache-Control", "no-store, no-cache, must-revalidate, private");
        resHeaders.set("Pragma", "no-cache");
        resHeaders.set("Expires", "0");
      } else if (normalizedPath === "/portal" || normalizedPath.startsWith("/portal/")) {
        // SPA 门户页面强制协商缓存，防止前端打包发版后客户端缓存旧 HTML 出现白屏死锁
        resHeaders.set("Cache-Control", "no-cache");
      }

      return new Response(response.body, {
        status: response.status,
        statusText: response.statusText,
        headers: resHeaders,
      });
    } catch (err) {
      return new Response("Gateway upstream connection timeout or reset.", {
        status: 502,
        headers: { "Content-Type": "text/plain; charset=utf-8" },
      });
    }
  },
};
