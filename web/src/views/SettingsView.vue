<template>
  <div class="space-y-4 max-w-4xl">
    <div
      class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 pb-2 border-b border-border/60"
    >
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">
            系统设置与核心解耦配置
          </h1>
          <Badge variant="outline" class="text-[10px] font-mono"> 零硬编码 / 官方标准托管 </Badge>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          配置 Xray Systemd 服务路径、订阅公共域名、Telegram 告警与管理员安全凭证
        </p>
      </div>

      <Button variant="default" size="sm" :loading="saving" @click="saveSystemSettings">
        <Save class="w-3.5 h-3.5 mr-1" />
        <span>{{ saving ? '保存中...' : '保存全部设置' }}</span>
      </Button>
    </div>

    <!-- 1. Xray 核心运行环境与 Systemd 托管设置 (零硬编码) -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <div class="flex items-center justify-between">
        <h2
          class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5"
        >
          <Cpu class="w-3.5 h-3.5 text-brand-400" />
          <span>① Xray 核心解耦与 Systemd 托管环境</span>
        </h2>
        <span class="text-[11px] text-muted-foreground font-mono">遵循 Linux 官方标准默认规范</span>
      </div>
      <p class="text-xs text-muted-foreground">
        面板与 Xray 核心完全解耦。在生产环境下面板通过 systemctl 管理 Xray
        守护进程，在此可动态修改核心路径与服务名。
      </p>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >Systemd 服务名称 (Service Name)</label
          >
          <input
            v-model="settings.xray_service_name"
            type="text"
            placeholder="xray"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            例如：<code>xray</code> 或 <code>xray.service</code>
          </p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1">Xray gRPC API 监听地址</label>
          <input
            v-model="settings.xray_grpc_addr"
            type="text"
            placeholder="127.0.0.1:8080"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            需与 config.json 中 api.listen 保持一致
          </p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >Xray 配置文件路径 (Config Path)</label
          >
          <input
            v-model="settings.xray_config_path"
            type="text"
            placeholder="/usr/local/etc/xray/config.json"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            官方路径：<code>/usr/local/etc/xray/config.json</code>
          </p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >Xray 二进制可执行文件 (Binary Path)</label
          >
          <input
            v-model="settings.xray_bin_path"
            type="text"
            placeholder="/usr/local/bin/xray"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            官方路径：<code>/usr/local/bin/xray</code>
          </p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1">GeoData 规则库存储目录</label>
          <input
            v-model="settings.xray_geodata_dir"
            type="text"
            placeholder="/usr/local/share/xray"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            官方路径：<code>/usr/local/share/xray</code>（存放 geoip.dat 与 geosite.dat）
          </p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >Xray 运行日志等级 (LogLevel)</label
          >
          <select
            v-model="settings.xray_log_level"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring cursor-pointer"
          >
            <option value="warning">warning (警告 - 默认推荐)</option>
            <option value="info">info (信息 - 包含连接生命周期)</option>
            <option value="debug">debug (调试 - 详细排错)</option>
            <option value="error">error (仅严重错误)</option>
            <option value="none">none (静音 - 关闭日志输出)</option>
          </select>
          <p class="text-[10px] text-muted-foreground mt-1">
            控制 Xray 核心运行与错误日志详细程度，修改后平滑重载生效
          </p>
        </div>
      </div>

      <!-- 本机 Master GeoData 规则库维护（紧邻 xray_geodata_dir） -->
      <div class="pt-3 border-t border-border/60 space-y-2.5">
        <div class="p-3 rounded-md bg-neutral-950 border border-border flex flex-col gap-2.5">
          <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
            <div class="flex items-center gap-2.5 min-w-0">
              <div class="p-2 rounded bg-card border border-border text-foreground shrink-0">
                <Database class="w-4 h-4 text-cyan-400" />
              </div>
              <div class="space-y-0.5 text-xs min-w-0">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="font-semibold text-foreground">
                    本机 Master GeoData 规则库 (geoip.dat &amp; geosite.dat)
                  </span>
                  <span
                    class="px-1.5 py-0.2 rounded text-[10px] font-mono bg-card border border-border text-cyan-400"
                  >
                    {{ geodataStatus?.platform || 'Xray Core' }}
                  </span>
                </div>
                <p class="text-[11px] text-muted-foreground font-mono break-all">
                  GeoIP:
                  {{
                    geodataStatus?.geoipExists
                      ? `${(geodataStatus.geoipSize / 1048576).toFixed(2)} MB`
                      : '未找到'
                  }}
                  | GeoSite:
                  {{
                    geodataStatus?.geositeExists
                      ? `${(geodataStatus.geositeSize / 1048576).toFixed(2)} MB`
                      : '未找到'
                  }}
                  | 路径：{{ geodataStatus?.targetDirectory || settings.xray_geodata_dir || './' }}
                </p>
                <p class="text-[10px] text-muted-foreground">
                  仅维护本机 Master 规则库；远端节点请前往「节点管理 -> 运维抽屉」按需下发更新。
                </p>
              </div>
            </div>

            <Button
              variant="secondary"
              size="sm"
              :loading="updatingGeo"
              :disabled="updatingGeo"
              @click="updateGeoData"
              class="shrink-0"
            >
              <RotateCw class="w-3.5 h-3.5 mr-1.5" :class="{ 'animate-spin': updatingGeo }" />
              <span>{{ updatingGeo ? '规则库升级中...' : '一键升级规则库' }}</span>
            </Button>
          </div>

          <div
            v-if="updatingGeo || (geoProgress.percentage > 0 && geoProgress.percentage < 100)"
            class="p-2.5 bg-card rounded-md border border-border space-y-1.5 transition-all"
          >
            <div class="flex items-center justify-between text-xs font-mono">
              <div class="flex items-center gap-2 min-w-0">
                <span
                  class="w-1.5 h-1.5 rounded-full shrink-0"
                  :class="updatingGeo ? 'bg-cyan-400 animate-ping' : 'bg-emerald-400'"
                ></span>
                <span class="text-muted-foreground text-[11px] truncate">
                  {{ geoProgress.message || '正在准备下载...' }}
                </span>
              </div>
              <span class="font-bold text-cyan-400 text-xs shrink-0">
                {{ geoProgress.percentage }}%
              </span>
            </div>

            <div
              class="w-full h-1.5 bg-neutral-900 rounded-full overflow-hidden border border-border/60"
            >
              <div
                class="h-full rounded-full bg-cyan-500 transition-all duration-300"
                :style="{ width: `${geoProgress.percentage}%` }"
              ></div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 2. 面板公共访问与订阅分发设置 -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <h2
        class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5"
      >
        <Globe class="w-3.5 h-3.5 text-cyan-400" />
        <span>② 面板公网访问与订阅地址配置</span>
      </h2>
      <p class="text-xs text-muted-foreground">
        指定用户获取订阅链接时使用的基础域名以及节点分享链接中的默认服务器外网连接地址。
      </p>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >面板公网访问 URL (Public URL)</label
          >
          <input
            v-model="settings.public_url"
            type="text"
            placeholder="http://IP:9000 或 https://panel.yourdomain.com"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            用于管理面板外网访问及默认聚合订阅 URL 前缀
          </p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >中立提取门户 URL (Portal URL / Worker 代理)</label
          >
          <input
            v-model="settings.portal_url"
            type="text"
            placeholder="https://sub.yourworker.workers.dev"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            安全提取码专属分发入口（推荐填写 Cloudflare Worker 地址以防封锁；留空自动跟随公网地址）
          </p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >节点连接地址 / 默认外网 IP</label
          >
          <input
            v-model="settings.sub_domain"
            type="text"
            placeholder="node1.yourdomain.com 或 服务器外网IP"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            若节点监听 0.0.0.0 时以此地址作为分享地址
          </p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >全局默认外部公网连接端口 (Public Port)</label
          >
          <input
            v-model.number="settings.public_port"
            type="number"
            placeholder="443"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            Nginx 前置 443 分流反代时，订阅链接默认下发的外部连接端口（默认 443）
          </p>
        </div>
      </div>
    </div>

    <!-- 3. 订阅导出与客户端展示配置 -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-4">
      <div class="flex items-center justify-between">
        <h2
          class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5"
        >
          <Sliders class="w-3.5 h-3.5 text-emerald-400" />
          <span>③ 订阅导出与客户端展示配置</span>
        </h2>
        <Badge variant="outline" class="text-[10px] font-mono"> 多端格式规范适配 </Badge>
      </div>
      <p class="text-xs text-muted-foreground">
        定制订阅下发时的顶部虚拟提示节点、自定义节点命名规则、HTTP 响应元信息以及 Clash / Sing-box
        等客户端配置参数。
      </p>

      <!-- 虚拟提示节点与置顶公告 -->
      <div class="p-3 rounded-md bg-neutral-950 border border-border/80 space-y-3">
        <div class="flex items-center justify-between">
          <div>
            <label class="text-xs font-medium text-foreground block"
              >聚合订阅顶部虚拟状态提示节点</label
            >
            <p class="text-[10px] text-muted-foreground mt-0.5">
              在客户端节点列表最顶端前置插入剩余流量与账号有效期提示项（兼容 Base64 / Clash /
              Sing-box，不影响实际代理连接）
            </p>
          </div>
          <input
            type="checkbox"
            v-model="settings.sub_show_info_nodes"
            class="rounded bg-neutral-900 border-border text-foreground focus:ring-0 cursor-pointer h-4 w-4"
          />
        </div>

        <div v-if="settings.sub_show_info_nodes" class="pt-2 border-t border-border/40">
          <label class="block text-muted-foreground text-xs font-medium mb-1"
            >置顶自定义公告内容 (选填)</label
          >
          <input
            v-model="settings.sub_custom_announcement"
            type="text"
            placeholder="例如：📢 欢迎使用，客服 TG 群: @yourgroup"
            class="w-full bg-neutral-900 border border-border rounded-md px-2.5 py-1.5 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            若填写，将在流量提示项上方生成
            <code>📢 公告: &lt;内容&gt;</code> 虚拟展示节点；留空则不生成公告。
          </p>
        </div>
      </div>

      <!-- 命名模板与 HTTP 响应元信息 -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
        <div class="sm:col-span-2 p-3 rounded-md bg-neutral-950 border border-border/80 space-y-3">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
            <div>
              <label for="sub-node-name-template-input" class="block text-foreground font-medium"
                >节点命名模板 (Node Name Template)</label
              >
              <p class="text-[10px] text-muted-foreground mt-0.5">
                默认
                <code>{remark}</code>
                为所见即所得，直接输出入站或分流线路的「订阅显示名称」；变量为空时自动清理空括号与多余分隔符
              </p>
            </div>

            <!-- 常用预设模板 -->
            <div class="flex items-center gap-1.5 flex-wrap shrink-0">
              <span class="text-[10px] text-muted-foreground font-mono">预设：</span>
              <button
                v-for="preset in NAME_TEMPLATE_PRESETS"
                :key="preset.template"
                type="button"
                @click="applyNameTemplatePreset(preset.template)"
                class="px-2 py-1 rounded border text-[10px] font-mono transition-colors cursor-pointer"
                :class="
                  (settings.sub_node_name_template || '{remark}') === preset.template
                    ? 'bg-emerald-500/15 border-emerald-500/40 text-emerald-300'
                    : 'bg-neutral-900 border-border text-muted-foreground hover:text-foreground hover:border-neutral-600'
                "
                :title="preset.template"
              >
                {{ preset.label }}
              </button>
            </div>
          </div>

          <input
            id="sub-node-name-template-input"
            ref="templateInputRef"
            v-model="settings.sub_node_name_template"
            type="text"
            placeholder="{remark}"
            class="w-full bg-neutral-900 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />

          <!-- 可点击插入变量芯片 -->
          <div class="space-y-1.5">
            <div class="text-[10px] text-muted-foreground font-mono">
              点击变量芯片插入模板（<code>{remark}</code> = 订阅显示名称（入站或分流线路名））：
            </div>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="chip in NAME_TEMPLATE_VARS"
                :key="chip.token"
                type="button"
                @click="insertTemplateVar(chip.token)"
                class="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-neutral-900 hover:bg-neutral-800 border border-border hover:border-neutral-600 text-[10px] font-mono transition-colors cursor-pointer"
                :title="chip.desc"
              >
                <span class="text-cyan-400">{{ chip.token }}</span>
                <span class="text-muted-foreground">{{ chip.label }}</span>
              </button>
            </div>
          </div>

          <!-- 实时渲染预览 -->
          <div class="p-2.5 rounded-md bg-card border border-border/80 space-y-1.5 font-mono">
            <div class="text-[10px] text-muted-foreground">
              实时渲染预览（含空值自动清理效果）：
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 text-[11px]">
              <div
                class="flex items-center justify-between gap-2 px-2.5 py-1.5 rounded bg-neutral-950 border border-border/60"
              >
                <span class="text-[10px] text-muted-foreground shrink-0">本机 Master (无地区)</span>
                <span class="text-foreground font-medium truncate">{{
                  templatePreview.master
                }}</span>
              </div>
              <div
                class="flex items-center justify-between gap-2 px-2.5 py-1.5 rounded bg-neutral-950 border border-border/60"
              >
                <span class="text-[10px] text-muted-foreground shrink-0">托管节点 (HK)</span>
                <span class="text-emerald-300 font-medium truncate">{{
                  templatePreview.remote
                }}</span>
              </div>
            </div>
          </div>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >客户端默认 uTLS 指纹 (Fingerprint)</label
          >
          <select
            v-model="settings.sub_default_fingerprint"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring cursor-pointer"
          >
            <option value="chrome">chrome (推荐)</option>
            <option value="firefox">firefox</option>
            <option value="safari">safari</option>
            <option value="random">random (随机)</option>
            <option value="ios">ios</option>
            <option value="android">android</option>
            <option value="edge">edge</option>
          </select>
          <p class="text-[10px] text-muted-foreground mt-1">
            TLS / REALITY 客户端握手伪装指纹缺省值
          </p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >订阅合集名称 (Profile-Title)</label
          >
          <input
            v-model="settings.sub_profile_title"
            type="text"
            placeholder="Xray Panel"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            注入至 HTTP 响应头 <code>Profile-Title</code>，客户端显示的订阅组名称
          </p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >自动更新间隔 (Profile-Update-Interval 小时)</label
          >
          <input
            v-model.number="settings.sub_update_interval"
            type="number"
            min="1"
            placeholder="24"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">
            注入至 HTTP 响应头，指示客户端按周期自动拉取更新 (默认 24 小时)
          </p>
        </div>
      </div>

      <!-- Clash 客户端导出高级参数 -->
      <div class="pt-2 border-t border-border/40 space-y-3">
        <h3 class="text-xs font-semibold text-foreground font-mono">
          Clash 客户端导出参数 (Clash Overwrite)
        </h3>
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3 text-xs">
          <div>
            <label class="block text-muted-foreground font-medium mb-1"
              >混合监听端口 (mixed-port)</label
            >
            <input
              v-model.number="settings.sub_clash_mixed_port"
              type="number"
              placeholder="7890"
              class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
            <p class="text-[10px] text-muted-foreground mt-1">Clash 本地混合代理监听端口</p>
          </div>

          <div>
            <label class="block text-muted-foreground font-medium mb-1">运行模式 (mode)</label>
            <select
              v-model="settings.sub_clash_mode"
              class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring cursor-pointer"
            >
              <option value="rule">rule (规则路由分流 - 推荐)</option>
              <option value="global">global (全局代理)</option>
              <option value="direct">direct (全部直连)</option>
            </select>
            <p class="text-[10px] text-muted-foreground mt-1">Clash 启动时的默认分流模式</p>
          </div>

          <div class="flex items-center pt-5 sm:pt-6">
            <label
              class="flex items-center gap-2 cursor-pointer text-muted-foreground hover:text-foreground"
            >
              <input
                type="checkbox"
                v-model="settings.sub_clash_allow_lan"
                class="rounded bg-neutral-900 border-border text-foreground focus:ring-0 cursor-pointer h-4 w-4"
              />
              <span class="text-xs text-foreground font-medium">允许局域网连接 (allow-lan)</span>
            </label>
          </div>
        </div>
      </div>
    </div>

    <!-- 4. Telegram 机器人运维与告警设置 -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <div class="flex items-center justify-between">
        <h2
          class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5"
        >
          <Send class="w-3.5 h-3.5 text-indigo-400" />
          <span>④ Telegram 机器人运维与告警配置</span>
        </h2>
        <Button variant="secondary" size="sm" :loading="testingTG" @click="testTelegram">
          <Send class="w-3 h-3 mr-1" />
          <span>{{ testingTG ? '发送中...' : '发送测试通知' }}</span>
        </Button>
      </div>

      <p class="text-xs text-muted-foreground">
        配置 Telegram Bot Token 与管理员 Chat
        ID，实时接收节点异常、流量超额、系统高负载告警，并支持在 TG 中使用
        <code>/status</code> 等指令。
      </p>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >Bot Token (从 @BotFather 获取)</label
          >
          <input
            v-model="settings.tg_bot_token"
            type="text"
            placeholder="123456789:ABCdefGhIJKlmNoPQRstuVWXyz"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1"
            >管理员 Chat ID (从 @userinfobot 获取)</label
          >
          <input
            v-model="settings.tg_admin_chat_id"
            type="text"
            placeholder="12345678"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>
      </div>
    </div>

    <!-- 5. 管理员密码修改 -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <h2
        class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5"
      >
        <Lock class="w-3.5 h-3.5 text-purple-400" />
        <span>⑤ 管理员密码安全修改</span>
      </h2>

      <form @submit.prevent="changePassword" class="space-y-3 max-w-md text-xs">
        <div>
          <label class="block text-muted-foreground mb-1 font-medium">当前旧密码</label>
          <input
            v-model="oldPassword"
            type="password"
            required
            placeholder="••••••••"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <div>
          <label class="block text-muted-foreground mb-1 font-medium">新密码</label>
          <input
            v-model="newPassword"
            type="password"
            required
            placeholder="••••••••"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <Button type="submit" variant="secondary" size="sm" :loading="changingPwd">
          {{ changingPwd ? '更新中...' : '修改管理员密码' }}
        </Button>
      </form>
    </div>

    <!-- 6. TOTP 双因素认证 (2FA) 安全加固 -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <div class="flex items-center justify-between">
        <h2
          class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5"
        >
          <ShieldCheck class="w-3.5 h-3.5 text-emerald-400" />
          <span>⑥ TOTP 双因素身份认证 (2FA)</span>
        </h2>
        <Badge :variant="totpEnabled ? 'default' : 'secondary'" class="text-[10px] font-mono">
          {{ totpEnabled ? '已启用 2FA 保护' : '未启用 2FA' }}
        </Badge>
      </div>

      <p class="text-xs text-muted-foreground">
        启用双因素认证后，每次登录除了需要输入密码外，还必须输入 Google Authenticator 或 1Password
        等应用生成的 6 位动态验证码，有效防范凭据泄露风险。
      </p>

      <div class="pt-1">
        <Button
          v-if="!totpEnabled"
          variant="default"
          size="sm"
          :loading="loading2FA"
          @click="startSetup2FA"
        >
          <QrCode class="w-3.5 h-3.5 mr-1" />
          <span>{{ loading2FA ? '加载中...' : '扫码绑定并开启 2FA' }}</span>
        </Button>

        <Button v-else variant="destructive" size="sm" @click="showDisableModal = true">
          <ShieldAlert class="w-3.5 h-3.5 mr-1" />
          <span>关闭 2FA 双因素保护</span>
        </Button>
      </div>
    </div>

    <!-- 2FA 绑定弹窗 -->
    <div
      v-if="showSetupModal"
      class="fixed inset-0 bg-black/75 z-50 flex items-center justify-center p-4"
    >
      <div class="bg-card border border-border rounded-lg p-5 max-w-sm w-full space-y-4 shadow-2xl">
        <div class="flex items-center justify-between pb-2 border-b border-border">
          <h3 class="text-sm font-bold text-foreground flex items-center gap-1.5">
            <QrCode class="w-4 h-4 text-emerald-400" />
            <span>绑定 Google 验证码 (2FA)</span>
          </h3>
          <button
            type="button"
            aria-label="关闭"
            @click="showSetupModal = false"
            class="text-muted-foreground hover:text-foreground text-xs"
          >
            ✕
          </button>
        </div>

        <p class="text-[11px] text-muted-foreground">
          请使用手机 Authenticator 扫描下方二维码，或手动输入 Secret 密钥：
        </p>

        <!-- QR Code -->
        <div class="bg-white p-3 rounded-md w-fit mx-auto shadow-inner">
          <qrcode-vue :value="totpSetupData.otpauthUrl" :size="160" level="M" />
        </div>

        <div class="bg-neutral-950 border border-border rounded-md p-2 text-center">
          <span class="text-[10px] uppercase text-muted-foreground block mb-0.5 font-mono"
            >Base32 Secret 密钥</span
          >
          <code class="text-xs font-mono font-bold text-emerald-400 select-all">{{
            totpSetupData.secret
          }}</code>
        </div>

        <div class="space-y-1.5">
          <label class="block text-xs text-foreground font-medium"
            >输入 App 生成的 6 位动态码确认：</label
          >
          <input
            v-model="setupPasscode"
            type="text"
            maxlength="6"
            placeholder="例如：123456"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-center text-sm font-mono text-foreground tracking-widest focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <div class="flex justify-end gap-2 pt-2 border-t border-border">
          <Button variant="secondary" size="sm" @click="showSetupModal = false">取消</Button>
          <Button
            variant="default"
            size="sm"
            :loading="enabling2FA"
            :disabled="setupPasscode.length !== 6"
            @click="confirmEnable2FA"
          >
            {{ enabling2FA ? '验证中...' : '确认开启 2FA' }}
          </Button>
        </div>
      </div>
    </div>

    <!-- 2FA 关闭确认弹窗 -->
    <div
      v-if="showDisableModal"
      class="fixed inset-0 bg-black/75 z-50 flex items-center justify-center p-4"
    >
      <div class="bg-card border border-border rounded-lg p-5 max-w-sm w-full space-y-4 shadow-2xl">
        <div class="flex items-center justify-between pb-2 border-b border-border">
          <h3 class="text-sm font-bold text-foreground flex items-center gap-1.5">
            <ShieldAlert class="w-4 h-4 text-rose-400" />
            <span>关闭 2FA 保护</span>
          </h3>
          <button
            type="button"
            aria-label="关闭"
            @click="showDisableModal = false"
            class="text-muted-foreground hover:text-foreground text-xs"
          >
            ✕
          </button>
        </div>

        <p class="text-xs text-muted-foreground">
          关闭 2FA 后登录将只需密码，请输入管理员密码以确认操作：
        </p>

        <div class="space-y-3 text-xs">
          <div>
            <label class="block text-muted-foreground mb-1 font-medium">管理员密码</label>
            <input
              v-model="disablePassword"
              type="password"
              placeholder="••••••••"
              class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
          <div>
            <label class="block text-muted-foreground mb-1 font-medium"
              >当前 6 位动态码 (必填)</label
            >
            <input
              v-model="disablePasscode"
              type="text"
              maxlength="6"
              placeholder="6 位动态码"
              class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono text-center tracking-widest focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
        </div>

        <div class="flex justify-end gap-2 pt-2 border-t border-border">
          <Button variant="secondary" size="sm" @click="showDisableModal = false">取消</Button>
          <Button
            variant="destructive"
            size="sm"
            :loading="disabling2FA"
            :disabled="!disablePassword || disablePasscode.length !== 6"
            @click="confirmDisable2FA"
          >
            {{ disabling2FA ? '关闭中...' : '确认关闭' }}
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onUnmounted } from 'vue'
import {
  Send,
  Globe,
  Lock,
  Cpu,
  Save,
  ShieldCheck,
  ShieldAlert,
  QrCode,
  Sliders,
  Database,
  RotateCw,
} from 'lucide-vue-next'
import QrcodeVue from 'qrcode.vue'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import { toast } from '../utils/toast'
import api from '../api'

const settings = ref<any>({
  xray_service_name: 'xray',
  xray_grpc_addr: '127.0.0.1:8080',
  xray_config_path: '/usr/local/etc/xray/config.json',
  xray_bin_path: '/usr/local/bin/xray',
  xray_geodata_dir: '/usr/local/share/xray',
  xray_log_level: 'warning',
  public_url: '',
  portal_url: '',
  sub_domain: '',
  public_port: 443,
  tg_bot_token: '',
  tg_admin_chat_id: '',
  sub_show_info_nodes: true,
  sub_custom_announcement: '',
  sub_node_name_template: '{remark}',
  sub_profile_title: 'Xray Panel',
  sub_update_interval: 24,
  sub_default_fingerprint: 'chrome',
  sub_clash_mixed_port: 7890,
  sub_clash_allow_lan: false,
  sub_clash_mode: 'rule',
})

const saving = ref(false)
const templateInputRef = ref<HTMLInputElement | null>(null)

const NAME_TEMPLATE_VARS = [
  { token: '{remark}', label: '订阅显示名称', desc: '订阅显示名称（入站或分流线路名）' },
  { token: '{region}', label: '节点地区', desc: '托管节点地区标签（Master 为空）' },
  { token: '{node}', label: '节点标识', desc: '托管节点标识 Name（Master 为空）' },
  { token: '{route}', label: '分流线路名', desc: 'VLESS 分流线路名称（单入站为空）' },
  { token: '{outbound}', label: '目标出站', desc: '分流线路目标出站 Tag' },
  { token: '{tag}', label: '入站 Tag', desc: '入站标识 Tag' },
  { token: '{protocol}', label: '协议', desc: '协议大写（VLESS/VMESS/TROJAN/SS）' },
  { token: '{port}', label: '端口', desc: '外部连接端口' },
  { token: '{index}', label: '序号', desc: '节点导出序号' },
]

const NAME_TEMPLATE_PRESETS = [
  { label: '所见即所得', template: '{remark}' },
  { label: '地区前缀', template: '[{region}] {remark}' },
  { label: '地区+协议', template: '{region} · {remark} ({protocol})' },
]

const applyNameTemplatePreset = (tpl: string) => {
  settings.value.sub_node_name_template = tpl
}

const insertTemplateVar = (token: string) => {
  const input = templateInputRef.value
  const current = String(settings.value.sub_node_name_template || '')
  if (
    !input ||
    typeof input.selectionStart !== 'number' ||
    typeof input.selectionEnd !== 'number'
  ) {
    settings.value.sub_node_name_template = current ? `${current} ${token}` : token
    return
  }
  const start = input.selectionStart
  const end = input.selectionEnd
  const next = current.slice(0, start) + token + current.slice(end)
  settings.value.sub_node_name_template = next
  void nextTick(() => {
    input.focus()
    const pos = start + token.length
    input.setSelectionRange(pos, pos)
  })
}

const renderNameTemplateSample = (
  tplRaw: string,
  vars: {
    remark: string
    tag: string
    protocol: string
    port: string
    index: string
    region: string
    node: string
    route: string
    outbound: string
  }
): string => {
  const tpl = (tplRaw || '').trim()
  if (!tpl || tpl === '{remark}') {
    return vars.remark || vars.tag
  }
  let out = tpl
    .replaceAll('{remark}', vars.remark)
    .replaceAll('{tag}', vars.tag)
    .replaceAll('{protocol}', vars.protocol)
    .replaceAll('{port}', vars.port)
    .replaceAll('{index}', vars.index)
    .replaceAll('{region}', vars.region)
    .replaceAll('{node}', vars.node)
    .replaceAll('{route}', vars.route)
    .replaceAll('{outbound}', vars.outbound)

  let prev = ''
  while (out !== prev) {
    prev = out
    out = out
      .replace(/\[\s*\]/g, '')
      .replace(/\(\s*\)/g, '')
      .replace(/【\s*】/g, '')
      .replace(/\s+/g, ' ')
      .trim()
  }
  out = out
    .replace(/^[-\s|·/]+/, '')
    .replace(/[-\s|·/]+$/, '')
    .replace(/\s([-|·/])(?:\s+\1)+(?=\s|$)/g, ' $1')
    .trim()
  return out || vars.remark || vars.tag
}

const templatePreview = computed(() => {
  const tpl = settings.value.sub_node_name_template || '{remark}'
  return {
    master: renderNameTemplateSample(tpl, {
      remark: '🇯🇵 日本东京原生',
      tag: 'vless-reality',
      protocol: 'VLESS',
      port: '443',
      index: '1',
      region: '',
      node: '',
      route: '🇯🇵 日本东京原生',
      outbound: 'direct',
    }),
    remote: renderNameTemplateSample(tpl, {
      remark: '香港高速 01',
      tag: 'hk-vless',
      protocol: 'VLESS',
      port: '8443',
      index: '2',
      region: 'HK',
      node: 'hk-edge-01',
      route: '',
      outbound: '',
    }),
  }
})

const testingTG = ref(false)
const oldPassword = ref('')
const newPassword = ref('')
const changingPwd = ref(false)

// 本机 Master GeoData 状态与轮询
const geodataStatus = ref<any>(null)
const updatingGeo = ref(false)
const geoProgress = ref<any>({
  isUpdating: false,
  percentage: 0,
  message: '',
  speedBps: 0,
})
let progressTimer: ReturnType<typeof setInterval> | null = null
let resetPercentTimer: ReturnType<typeof setTimeout> | null = null
let progressInFlight = false
let isUnmounted = false

const stopGeoTimers = () => {
  if (progressTimer) {
    clearInterval(progressTimer)
    progressTimer = null
  }
  if (resetPercentTimer) {
    clearTimeout(resetPercentTimer)
    resetPercentTimer = null
  }
  progressInFlight = false
}

const fetchGeoStatus = async () => {
  try {
    const res: any = await api.get('/geodata/status')
    if (!isUnmounted) {
      geodataStatus.value = res
    }
  } catch (err) {
    console.error(err)
  }
}

const pollGeoProgress = async () => {
  if (isUnmounted || progressInFlight) return
  progressInFlight = true
  try {
    const res: any = await api.get('/geodata/progress')
    if (isUnmounted) return
    if (res) {
      geoProgress.value = res
      if (!res.isUpdating) {
        if (progressTimer) {
          clearInterval(progressTimer)
          progressTimer = null
        }
        updatingGeo.value = false
        if (res.step === 'done') {
          toast.success(res.message || 'GeoData 规则库已更新')
          await fetchGeoStatus()
          if (!isUnmounted) {
            if (resetPercentTimer) clearTimeout(resetPercentTimer)
            resetPercentTimer = setTimeout(() => {
              if (!isUnmounted) {
                geoProgress.value.percentage = 0
              }
            }, 4000)
          }
        } else if (res.step === 'error') {
          toast.error(res.message || '更新规则库失败')
        }
      }
    }
  } catch (e) {
    console.error('poll progress failed', e)
  } finally {
    progressInFlight = false
  }
}

const updateGeoData = async () => {
  if (
    !confirm(
      '确定在线更新本机 Master 的 GeoIP / GeoSite 规则库吗？更新完成后将自动重载 Xray 核心。'
    )
  ) {
    return
  }
  stopGeoTimers()
  updatingGeo.value = true
  geoProgress.value = { isUpdating: true, percentage: 5, message: '正在启动后台下载引擎...' }
  try {
    await api.post('/geodata/update')
    if (isUnmounted) return
    progressTimer = setInterval(pollGeoProgress, 500)
  } catch (err: any) {
    if (!isUnmounted) {
      updatingGeo.value = false
      toast.error('触发更新失败：' + err)
    }
  }
}

// 2FA 状态
const totpEnabled = ref(false)
const showSetupModal = ref(false)
const showDisableModal = ref(false)
const loading2FA = ref(false)
const enabling2FA = ref(false)
const disabling2FA = ref(false)
const totpSetupData = ref({ secret: '', otpauthUrl: '' })
const setupPasscode = ref('')
const disablePassword = ref('')
const disablePasscode = ref('')

const fetchAdminInfo = async () => {
  try {
    const res: any = await api.get('/auth/info')
    if (res) {
      totpEnabled.value = !!res.totpEnabled
    }
  } catch (err) {
    console.error(err)
  }
}

const fetchSettings = async () => {
  try {
    const res: any = await api.get('/settings')
    if (res) {
      settings.value = {
        ...settings.value,
        ...res,
      }
      if (res.sub_show_info_nodes !== undefined) {
        settings.value.sub_show_info_nodes =
          res.sub_show_info_nodes === true ||
          res.sub_show_info_nodes === 'true' ||
          res.sub_show_info_nodes === '1'
      }
      if (res.sub_clash_allow_lan !== undefined) {
        settings.value.sub_clash_allow_lan =
          res.sub_clash_allow_lan === true ||
          res.sub_clash_allow_lan === 'true' ||
          res.sub_clash_allow_lan === '1'
      }
      if (res.sub_update_interval !== undefined) {
        settings.value.sub_update_interval = Number(res.sub_update_interval) || 24
      }
      if (res.sub_clash_mixed_port !== undefined) {
        settings.value.sub_clash_mixed_port = Number(res.sub_clash_mixed_port) || 7890
      }
    }
    // 若未配置 public_url 或仍为默认 127.0.0.1，自动根据当前浏览器地址推荐
    if (!settings.value.public_url || settings.value.public_url.includes('127.0.0.1')) {
      settings.value.public_url = `${window.location.protocol}//${window.location.host}`
    }
  } catch (err) {
    console.error(err)
  }
}

const saveSystemSettings = async () => {
  saving.value = true
  try {
    await api.post('/settings', settings.value)
    toast.success('系统与解耦设置已保存并生效')
    await fetchSettings()
    await fetchGeoStatus()
  } catch (err: any) {
    toast.error('保存失败：' + err)
  } finally {
    saving.value = false
  }
}

const testTelegram = async () => {
  if (!settings.value.tg_bot_token || !settings.value.tg_admin_chat_id) {
    toast.warning('请先填写 Bot Token 与 Admin Chat ID 并保存')
    return
  }
  testingTG.value = true
  try {
    await api.post('/settings/test-telegram')
    toast.success('测试消息已发送至 Telegram，请在客户端查看')
  } catch (err: any) {
    toast.error('发送失败：' + err)
  } finally {
    testingTG.value = false
  }
}

const changePassword = async () => {
  if (newPassword.value.length < 6) {
    toast.warning('新密码长度不能少于 6 位')
    return
  }
  changingPwd.value = true
  try {
    await api.post('/auth/change-password', {
      oldPassword: oldPassword.value,
      newPassword: newPassword.value,
    })
    toast.success('管理员密码已修改，请牢记新密码')
    oldPassword.value = ''
    newPassword.value = ''
  } catch (err: any) {
    toast.error('修改失败：' + err)
  } finally {
    changingPwd.value = false
  }
}

const startSetup2FA = async () => {
  loading2FA.value = true
  try {
    const res: any = await api.get('/auth/2fa/setup')
    totpSetupData.value = {
      secret: res.secret,
      otpauthUrl: res.otpauthUrl,
    }
    setupPasscode.value = ''
    showSetupModal.value = true
  } catch (err: any) {
    toast.error('获取 2FA 密钥失败：' + err)
  } finally {
    loading2FA.value = false
  }
}

const confirmEnable2FA = async () => {
  if (setupPasscode.value.length !== 6) {
    toast.warning('请输入完整的 6 位动态验证码')
    return
  }
  enabling2FA.value = true
  try {
    const res: any = await api.post('/auth/2fa/enable', {
      secret: totpSetupData.value.secret,
      passcode: setupPasscode.value,
    })
    toast.success(res.message || '2FA 双因素认证已开启')
    totpEnabled.value = true
    showSetupModal.value = false
  } catch (err: any) {
    toast.error('开启失败：' + (typeof err === 'string' ? err : '验证码不正确'))
  } finally {
    enabling2FA.value = false
  }
}

const confirmDisable2FA = async () => {
  if (!disablePassword.value) {
    toast.warning('请输入密码确认')
    return
  }
  if (!disablePasscode.value || disablePasscode.value.length !== 6) {
    toast.warning('请输入 6 位 2FA 动态码')
    return
  }
  disabling2FA.value = true
  try {
    const res: any = await api.post('/auth/2fa/disable', {
      password: disablePassword.value,
      passcode: disablePasscode.value,
    })
    toast.success(res.message || '2FA 已关闭')
    totpEnabled.value = false
    showDisableModal.value = false
    disablePassword.value = ''
    disablePasscode.value = ''
  } catch (err: any) {
    toast.error('关闭失败：' + (typeof err === 'string' ? err : '密码或验证码错误'))
  } finally {
    disabling2FA.value = false
  }
}

onMounted(() => {
  fetchSettings()
  fetchAdminInfo()
  fetchGeoStatus()
})

onUnmounted(() => {
  isUnmounted = true
  stopGeoTimers()
})
</script>
