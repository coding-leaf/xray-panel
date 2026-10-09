# Implementation Plan: Inbound Traffic User Binding

## Ordered Checklist

- [x] 1. **Domain & Database Migration**
  - 修改 `internal/domain/inbound.go`，为 `Inbound` 结构体增加 `BoundUserEmail` 字段。
  - 检查并在单元测试中验证 GORM AutoMigrate 及序列化正常。

- [x] 2. **ConfigService Inbound Validation**
  - 在 `internal/service/config_service.go` 中，增加 `BoundUserEmail` 的存在性校验（若非空则确保存在该用户）。
  - 为 `ConfigService` 补充针对 `BoundUserEmail` 校验的单元测试。

- [x] 3. **TrafficSyncJob Logic Enhancement**
  - 在 `internal/delivery/cron/traffic_sync.go` 中：
    - 读取 Inbound 列表构建 `tag -> BoundUserEmail` 映射表。
    - 在处理 `domain.TrafficStatTypeInbound` 时，如果对应 tag 存在 `BoundUserEmail`，将增量累加至对应用户的 `userDeltas` 和实时速率 `userDeltaUp/Down` 中。
  - 在 `internal/delivery/cron/traffic_sync_test.go` 中新增测试用例：
    - 模拟 `inbound>>>local>>>traffic>>>...`，验证绑定用户的 `Up/Down` 流量被正确累加至用户表与 TrafficLog。

- [x] 4. **Web Frontend Adaptation**
  - 更新 `web/src/views/inbounds/types.ts`，增加 `boundUserEmail` 字段及其表单清洗与序列化逻辑。
  - 更新 `web/src/views/inbounds/components/InboundFormDrawer.vue`，增加绑定用户选择项。
  - 更新 `web/src/views/inbounds/components/InboundTable.vue` 和 `InboundDetailDrawer.vue`，展示绑定用户。
  - 更新 `web/src/mock/storage.ts` 与 mock 逻辑以支持本地 mock 预览。

- [x] 5. **Quality Gates & Verification**
  - 运行后端测试：`go test ./...` 与 `go vet ./...`。
  - 运行前端构建：`npm run build`。
