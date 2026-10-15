# 实施计划 (Implementation Plan)

## 任务目标

废除 `compiler.go` 中对 `vlessRoute` 的静默过滤，确立“机制极简、策略放权”的路由编译体系，支持通过自定义规则与一键向导精准编排跨机节点分流。

---

## 阶段一：后端编译器放权改造与单测验证 (Backend)

- [x] **1.1 审查与移除 `compiler.go` 静默过滤**
  - 文件：`internal/adapter/xray/compiler.go`
  - 移除 Layer 2 循环中的 `if r.VlessRoute != "" { continue }`；
  - 确保 `r.VlessRoute` 正确赋值给 `XrayRoutingRule.VlessRoute`；
  - 确认除 `inboundTag: ["api"]` 外无任何硬编码的端口/协议阻断。

- [x] **1.2 编写与更新单测 `compiler_test.go`**
  - 文件：`internal/adapter/xray/compiler_test.go`
  - 新增测试用例：验证包含 `vlessRoute` 的自定义规则被忠实编译到 Layer 2；
  - 验证自定义规则与 SubRoute 自动兜底规则的先后层级关系；
  - 验证当用户自定义 `vlessRoute: "2"` 时，该规则出现在全局 `geosite:cn` 前方能成功优先命中。

- [x] **1.3 执行后端质量门禁**
  - 运行 `mise x -- go test ./...` 确保全部单测通过；
  - 运行 `mise x -- go vet ./...` 确保静态检查无报错；
  - 运行 `mise x -- go build .` 确保二进制构建成功。

---

## 阶段二：前端 UI 体验验证与向导对齐 (Frontend)

- [x] **2.1 审查 `web/src/views/RoutingView.vue`**
  - 验证一键向导（广告、私有 IP、BT、SMTP 25、国内分流）生成的规则形态；
  - 验证规则列表中对 `vlessRoute` 的展示、编辑与拖拽排序体验；
  - 确保向导仅作为纯客户端模版生成，无硬编码副作用。

- [x] **2.2 执行前端质量门禁**
  - 在 `web/` 目录下运行 `mise x -- npm run build`；
  - 确保生产构建 100% 成功，类型检查无报错。

---

## 阶段三：综合验收与防线审查 (Review & Finish)

- [x] **3.1 跨节点分流场景回归**
  - 验证 JP 网关场景下，显式指定 US 出口（`vlessRoute: 2`）的流量不再被 `geosite:cn` 意外分流到本地 `warp-out` 的编排逻辑正确性。
- [x] **3.2 全量门禁验收**
  - 后端：`mise x -- go test ./...` + `mise x -- go vet ./...` + `mise x -- go build .`
  - 前端：`cd web && mise x -- npm run build`
