# Xray Decoupled Panel (独立运维监控与订阅分发面板)

与 Xray 核心完全解耦的单机运维监控与订阅分发面板，采用 **Clean Architecture** 分层设计，单文件可执行二进制部署。

## 功能特性
- **完全解耦**：仅通过 Xray 官方 gRPC API（HandlerService, StatsService）与 systemd 信号控制，不侵入 Xray 核心进程。
- **单二进制运行**：Vue 3 前端编译资产通过 `//go:embed` 静态嵌入 Go 二进制，零外部依赖。
- **系统与流量监控**：CPU、内存、磁盘与实时网络 I/O 速率采集，周期采集用户/节点流量。
- **订阅与节点分发**：独立安全 Token 订阅分发（`/api/sub/:token`），支持 Xray 标准格式分享链接与二维码。
- **双模配置管理**：可视化入站参数维护 + 完整 `config.json` 在线编辑器（带 `xray -test` 严格语法校验与平滑热重载）。
- **Telegram Bot 运维互动**：交互式指令（`/status`, `/traffic`, `/sub`, `/restart`）与流量超额/系统高负载主动告警。

## 快速运行
```bash
# 启动面板
.\panel.exe -port 9000 -xray-config ../server-configs/xray/config.json -xray-grpc 127.0.0.1:8080
```
- 默认管理员账号：`admin`
- 默认管理员密码：`admin123`
