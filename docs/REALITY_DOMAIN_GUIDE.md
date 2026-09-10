# Xray VLESS-REALITY 伪装域名选型、避坑指南与备用候选库

本文档用于归档 REALITY 协议下目标网站（`dest`）与主机名（`serverNames`）的选型规范、实测合格的备用候选域名以及本地探测工具的使用方法，方便后续更换机房或维护节点时随时查阅。

---

## 一、 当前配置评估结论

* **当前在用目标**：`dest: "www.titech.ac.jp:443"`, `serverNames: ["www.titech.ac.jp"]`
* **实测指标**：
  * **TLS 1.3**：支持（协商套件：`TLS_AES_256_GCM_SHA384`）
  * **ALPN HTTP/2**：支持（协商返回：`h2`）
  * **CDN 状态**：无 Cloudflare 特征，直连日本学术骨干网（SINET）
  * **到本机 RTT**：**~0.89 ms**（同在东京，延迟极小）
* **结论**：**保持现状即可，当前配置处于顶级最优状态，无需更换。**

---

## 二、 实测合格备用候选域名库（随取随用）

以下均为在当前日本网络环境下通过真实协议握手与延迟测速验证合格的目标，后续如需更换，可直接复制对应的配置代码块覆盖至 [`config.json`](../config.json) 的 `realitySettings`：

### 候选 1：东京科学大学（当前在用，最优）
```json
"realitySettings": {
    "dest": "www.titech.ac.jp:443",
    "serverNames": [
        "www.titech.ac.jp"
    ],
    "privateKey": "...",
    "shortIds": ["", "0123456789abcdef"]
}
```
* **客户端 `serverName`**：`www.titech.ac.jp`
* **实测延迟**：~0.9 ms | **协议**：TLS 1.3 / HTTP/2

---

### 候选 2：xTom 日本基础设施镜像站（算法自动发现推荐）
```json
"realitySettings": {
    "dest": "mirrors.xtom.jp:443",
    "serverNames": [
        "mirrors.xtom.jp"
    ],
    "privateKey": "...",
    "shortIds": ["", "0123456789abcdef"]
}
```
* **客户端 `serverName`**：`mirrors.xtom.jp`
* **实测延迟**：~7.3 ms | **协议**：TLS 1.3 / HTTP/2

---

### 候选 3：千叶大学（日本樱花网络 Sakura Internet）
```json
"realitySettings": {
    "dest": "www.chiba-u.ac.jp:443",
    "serverNames": [
        "www.chiba-u.ac.jp"
    ],
    "privateKey": "...",
    "shortIds": ["", "0123456789abcdef"]
}
```
* **客户端 `serverName`**：`www.chiba-u.ac.jp`
* **实测延迟**：~1.4 ms | **协议**：TLS 1.3 / HTTP/2

---

### 候选 4：筑波大学（东京节点）
```json
"realitySettings": {
    "dest": "www.tsukuba.ac.jp:443",
    "serverNames": [
        "www.tsukuba.ac.jp"
    ],
    "privateKey": "...",
    "shortIds": ["", "0123456789abcdef"]
}
```
* **客户端 `serverName`**：`www.tsukuba.ac.jp`
* **实测延迟**：~2.2 ms | **协议**：TLS 1.3 / HTTP/2

---

## 三、 `serverNames` 与 `dest` 核心规则备忘（防踩坑）

1. **服务端 `serverNames` 是数组，但严禁跨站混填**：
   * 数组里的所有域名必须是 `dest` 网站证书中的合法备用名称（SAN）。
   * ❌ **反例**：不能把 `["www.titech.ac.jp", "mirrors.xtom.jp"]` 写在一起。当遭遇主动探测时，服务器会把连接转发给 `dest`，由于证书与 SNI 不一致，伪装会立刻暴露。
2. **客户端 `serverName` 必须是单字符串**：
   * 底层 TLS 握手规范限制，客户端一次只能发一个 SNI。
   * 客户端填写的字符串必须是服务端 `serverNames` 列表里的其中一个。
3. **黄金实践**：
   * **单一对应**：让 `dest` 的主机名与 `serverNames` 填完全一样的单个域名，最省心且隐蔽性最高。

---

## 四、 本地探测工具箱（存放在 `temp/` 目录下）

后续如果将服务器迁移到其他城市（如**大阪、福冈、法兰克福、新加坡**等），可直接调用本地已准备好的全自动工具链：

### 1. 全自动零猜测自适应发现引擎（最推荐）
```bash
./temp/auto_discover_nearby.sh
```
* **作用**：自动探测当前服务器真实 IP、城市和所属 BGP ASN，并发向邻近与权威镜像源测速，输出当前物理距离最近的排名前列域名，无需人脑猜测当地网站。

### 2. 单域名深度体检脚本
```bash
./temp/check_domain.sh <域名>

# 示例:
./temp/check_domain.sh mirrors.xtom.jp
```
* **作用**：全方位检查 DNS、Cloudflare/CDN 识别、TLS 1.3 支持、ALPN HTTP/2 协商，并给出推荐配置。

### 3. 同网段邻居嗅探脚本
```bash
./temp/scan_subnet.sh [可选网段CIDR]
```
* **作用**：低并发扫描同机房邻居 IP，探测是否有人在运行带 SSL 证书的 Web 站点。
