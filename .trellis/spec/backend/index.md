# Backend Development Guidelines - xray-panel

> xray-panel 后端开发规范、分层架构与质量契约。

---

## 架构拓扑 (Layering Architecture)

系统遵循严格的分层架构设计：

```
internal/
├── config/               # 启动配置装载 (flag + 环境变量，如 -log-level / LOG_LEVEL)
├── domain/               # 纯领域模型与无副作用业务逻辑 (状态计算、路由计算、票据规则)
├── service/              # 跨领域用例编排、事务管理与业务流控制
├── adapter/              # 外部技术适配层
│   ├── monitor/          # 系统资源指标采集 (gopsutil)
│   ├── reality/          # REALITY 目标探测与健康度评估
│   ├── repository/       # GORM + SQLite 持久化仓储实现
│   ├── telegram/         # Telegram Bot 运维交互与会话轮询
│   ├── xray/             # Xray-core gRPC 交互与 XrayCompiler 配置编译器
│   └── xray/proto/       # Xray 原生 Protobuf 客户端定义
├── delivery/
│   ├── http/             # HTTP 路由、REST API Handler 与中间件 (JWT, 鉴权, 限流)
│   └── cron/             # 定时轮询任务 (TrafficSyncJob, RealitySyncJob)
├── protocol/             # 节点协议抽象、动态多格式注册表 (Clash, Sing-Box, Base64)
├── sub/                  # 聚合订阅导出业务编排
├── app/                  # 全局应用生命周期有序编排 (errgroup 优雅停机)
└── pkg/                  # 基础设施通用工具库 (logger, jwt, totp, cache, jsonc)
```

---

## 核心规范文件索引

| 规约文档 | 覆盖范围与说明 | 状态 |
|---------|----------------|------|
| [Directory Structure](./directory-structure.md) | 模块分工、目录边界与调用链规范 | Active |
| [Database Guidelines](./database-guidelines.md) | 纯 Go SQLite、WAL 模式、GORM 事务与并发安全 | Active |
| [Error Handling](./error-handling.md) | 显式错误传播、禁止静默吞错与 HTTP 映射 | Active |
| [Quality Guidelines](./quality-guidelines.md) | 并发安全、KISS 极简规范与反过度设计红线 | Active |
| [Logging Guidelines](./logging-guidelines.md) | 结构化日志、等级划分与敏感凭据脱敏防线 | Active |
