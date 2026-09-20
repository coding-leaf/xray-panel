---
name: subagent-stats
description: 统计与查看 OpenCode 最近一次主任务会话（或指定会话）派生出的全部子代理（planner, builder, reviewer 等）Token 开销、思考链、Prompt 缓存命中率与耗时。在用户询问子代理消耗、Token 用量或性能分析时激活,强依赖opencode底层。
---

# OpenCode Subagent Token & Cache Stats Skill

本技能指导 OpenCode 快速查询并汇报当前或最近任务中子代理（Subagents）的 Token 消耗、Reasoning 开销与 Prompt Cache 命中率。

## 1. 核心执行指令

直接在终端执行项目内置工具：
```bash
# 默认查看当前/最近的主任务会话（包含主会话与全部派生子代理度量）
python3 tooling/subagent_stats.py

# 仅统计子代理（过滤无子代理的会话，排除主会话行）
python3 tooling/subagent_stats.py -S

# 查看最近 N 个主任务会话
python3 tooling/subagent_stats.py -n 3

# 查看指定 session_id 的子代理
python3 tooling/subagent_stats.py -s <session_id>
```

## 2. 字段度量与计算定义

* **Input (Pmt)**：模型未命中缓存的实际输入 Prompt Token。
* **Cache Read**：上下文缓存命中（Prompt Cache Read）Token。
* **Cache Hit %**：缓存命中率，计算公式：
  $$\text{Cache Hit \%} = \frac{\text{Cache Read}}{\text{Input} + \text{Cache Read}} \times 100\%$$
* **Output (Cpl)**：模型生成补全的 Completion Token。
* **Reasoning**：模型思考链消耗的 Token。
* **Duration**：子代理从启动到退出的执行时间。

## 3. 汇报指引

执行后直接将终端的美化表格输出或简短总结反馈给用户，无需额外生成长篇 markdown 文档。
