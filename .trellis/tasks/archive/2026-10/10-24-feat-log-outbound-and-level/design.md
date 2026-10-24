# 新增出站日志过滤与日志等级全栈能力 - 技术设计 (Design)

## 1. 架构流向与数据模型设计

### 1.1 日志过滤拓展
- **`domain.LogFilter`**：
  ```go
  type LogFilter struct {
      InboundTag  string
      OutboundTag string
      Level       string
      Keyword     string
  }
  ```
- **Xray 日志读取适配器 (`log_reader.go`)**：
  - `matchOutboundTagBytes(line []byte, tag string) bool`：
    扫描 `[` 与 `]` 之间的路由部分，匹配 `-> <outbound>` 或 `>> <outbound>`。若行包含该出口标签，判定匹配成功。
  - `matchLevelBytes(line []byte, level string) bool`：
    扫描形如 `[Info]`、`[Warning]`、`[Error]`、`[Debug]` 的标记并大小写无关比对。

### 1.2 Xray 核心运行日志等级可配性
- **编译器 (`compiler.go`)**：
  ```go
  type XrayCompiler struct {
      grpcPort int
      registry *ProtocolRegistry
      logLevel string
  }
  func (c *XrayCompiler) SetLogLevel(lvl string)
  func (c *XrayCompiler) GetLogLevel() string
  ```
- **系统设置服务 (`setting_service.go`)**：
  - 键名：`xray_log_level`，合法值 `debug | info | warning | error | none`，缺省 `warning`；
  - 允许注入 `ConfigService` 或 `recompile` 管道：当 `xray_log_level` 改变时，更新 compiler 并调用 `RecompileAndApply` 触发平滑热更新；
  - 面板启动时（`main.go`）读取持久化值并同步给 compiler。

### 1.3 前端设计
- **`LogsView.vue`**：
  - 增加 `selectedOutbound` 响应式状态；
  - 访问模式工具栏增加出站选择下拉框；
  - 点击表格出站 Badge 可触发 `selectedOutbound = tag` 联动过滤；
  - 错误模式下，切换级别除保留前端快速响应外，带入请求参数提升后端扫描效率。
- **`SettingsView.vue`**：
  - 在核心解耦与 Systemd 托管环境卡片内增加 LogLevel 下拉选择器；
  - 保存时向 `/settings` 提交。

## 2. 性能与并发安全
- `matchOutboundTagBytes` 与 `matchLevelBytes` 保持零堆内存分配（纯切片比对），契合 64KB Chunk Buffer 反向块扫描机制。
- `compiler.logLevel` 读写通过互斥或原子保护，确保配置重载与并发查询安全。
