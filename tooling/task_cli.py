#!/usr/bin/env python3
"""Xray Panel SDLC Task Management CLI (task_cli).

A command-line tool for developers and AI agents to manage active SDLC tasks,
automate artifact generation, perform gate checks, and archive deliverables.
"""

from __future__ import annotations

import argparse
import datetime
import re
import shutil
import sys
from dataclasses import dataclass
from pathlib import Path

# 确保直接运行脚本时可定位根模块
_PROJECT_ROOT = Path(__file__).resolve().parent.parent
if str(_PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(_PROJECT_ROOT))


VALID_STAGES = ["Plan", "Design", "Build", "Test", "Review", "Deploy"]
STAGE_ORDER = {stage: i for i, stage in enumerate(VALID_STAGES)}
VALID_STATUSES = ["in_progress", "blocked", "ready_for_qa", "ready_for_review"]
VALID_RISKS = ["Tier 1", "Tier 2", "Tier 3"]


def _extract_field(body: str, name: str, default: str = "") -> str:
    m = re.search(rf"^-\s+{name}:\s*(.*?)$", body, re.MULTILINE)
    return m.group(1).strip() if m else default


@dataclass
class TaskCard:
    task_id: str
    stage: str = "Plan"
    status: str = "in_progress"
    risk: str = "Tier 2"
    owner: str = "Dev"
    created: str = ""
    next_action: str = "编写 intent.md 与初步方案"
    blocked: str = "None"
    sdlc_path: str = ""

    def to_markdown(self) -> str:
        lines = [
            f"## {self.task_id}",
            f"- Stage: {self.stage}",
            f"- Status: {self.status}",
            f"- Risk: {self.risk}",
            f"- Owner: {self.owner}",
        ]
        if self.created:
            lines.append(f"- Created: {self.created}")
        lines.extend([
            f"- Next: {self.next_action}",
            f"- Blocked: {self.blocked}",
            f"- SDLC: {self.sdlc_path}",
        ])
        return "\n".join(lines) + "\n"


class TaskManager:
    def __init__(self, workspace_root: Path | None = None):
        if workspace_root is None:
            # 默认以当前脚本所在目录的上级目录作为项目根目录
            self.root = Path(__file__).resolve().parent.parent
        else:
            self.root = Path(workspace_root).resolve()

        self.docs_dir = self.root / "docs"
        self.sdlc_dir = self.docs_dir / "sdlc"
        self.template_dir = self.sdlc_dir / "_template"
        self.active_tasks_file = self.docs_dir / "ACTIVE_TASKS.md"
        self.archive_file = self.sdlc_dir / "ARCHIVE.md"

    def _ensure_active_tasks_file(self) -> None:
        if not self.active_tasks_file.exists():
            self.active_tasks_file.parent.mkdir(parents=True, exist_ok=True)
            initial_content = (
                "# Xray Panel Active Tasks (当前活跃任务索引)\n\n"
                "> **定位说明**：  \n"
                "> 本文件是当前活跃任务的轻量导航索引，**不是任务事实的唯一来源**。  \n"
                "> 若本索引与底层 `docs/sdlc/<task-id>/` 工件或实际工作区状态冲突，一律以实际客观代码与工件为准，并立即纠偏本索引。  \n"
                "> 任务完成并通过验收后，从本文件移除并沉淀至 `docs/sdlc/ARCHIVE.md`。\n\n"
                "---\n\n"
                "(暂无活跃任务)\n"
            )
            self.active_tasks_file.write_text(initial_content, encoding="utf-8")

    def _ensure_archive_file(self) -> None:
        if not self.archive_file.exists():
            self.archive_file.parent.mkdir(parents=True, exist_ok=True)
            initial_content = (
                "# Xray Panel SDLC Archive (已归档成果与变更审计)\n\n"
                "> **定位说明**：  \n"
                "> 本文件是已交付任务的成果索引大表。  \n"
                "> 本表仅作成果与证据索引，不重复记录任务的完整设计细节（完整过程由对应的 `docs/sdlc/<task-id>/` 目录留存）。\n\n"
                "---\n\n"
                "| Task | Outcome | Final Stage | Commit/PR | Verification | Completed |\n"
                "|---|---|---|---|---|---|\n"
            )
            self.archive_file.write_text(initial_content, encoding="utf-8")

    def parse_active_tasks(self) -> dict[str, TaskCard]:
        self._ensure_active_tasks_file()
        raw_content = self.active_tasks_file.read_text(encoding="utf-8")
        # 过滤掉 HTML 注释块，避免模板示例被误识别为实际任务
        clean_content = re.sub(r"<!--.*?-->", "", raw_content, flags=re.DOTALL)
        tasks: dict[str, TaskCard] = {}

        # 匹配 ## <task_id> 块
        pattern = re.compile(r"^##\s+([^\n]+)\n(.*?)(?=\n##|\Z)", re.MULTILINE | re.DOTALL)
        for match in pattern.finditer(clean_content):
            task_id = match.group(1).strip()
            body = match.group(2)

            card = TaskCard(
                task_id=task_id,
                stage=_extract_field(body, "Stage", "Plan"),
                status=_extract_field(body, "Status", "in_progress"),
                risk=_extract_field(body, "Risk", "Tier 2"),
                owner=_extract_field(body, "Owner", "Dev"),
                created=_extract_field(body, "Created", ""),
                next_action=_extract_field(body, "Next", ""),
                blocked=_extract_field(body, "Blocked", "None"),
                sdlc_path=_extract_field(body, "SDLC", f"docs/sdlc/{task_id}/"),
            )
            tasks[task_id] = card

        return tasks

    def _save_active_tasks(self, tasks: dict[str, TaskCard]) -> None:
        self._ensure_active_tasks_file()
        lines = [
            "# Xray Panel Active Tasks (当前活跃任务索引)",
            "",
            "> **定位说明**：  ",
            "> 本文件是当前活跃任务的轻量导航索引，**不是任务事实的唯一来源**。  ",
            "> 若本索引与底层 `docs/sdlc/<task-id>/` 工件或实际工作区状态冲突，一律以实际客观代码与工件为准，并立即纠偏本索引。  ",
            "> 任务完成并通过验收后，从本文件移除并沉淀至 `docs/sdlc/ARCHIVE.md`。",
            "",
            "---",
            "",
        ]

        if not tasks:
            lines.append("(暂无活跃任务)\n")
        else:
            for card in tasks.values():
                lines.append(card.to_markdown().strip())
                lines.append("")

        self.active_tasks_file.write_text("\n".join(lines).strip() + "\n", encoding="utf-8")

    def _materialize_doc(self, task_id: str, doc_name: str, title: str, owner: str, risk_str: str, created_at: str | None = None) -> None:
        task_artifact_dir = self.sdlc_dir / task_id
        task_artifact_dir.mkdir(parents=True, exist_ok=True)
        target_file = task_artifact_dir / doc_name
        if target_file.exists():
            return

        if created_at is None:
            created_at = datetime.datetime.now(datetime.timezone.utc).astimezone().strftime("%Y-%m-%d %H:%M")

        tpl_map = {
            "intent.md": ("intent.template.md", "[简明任务或缺陷标题]", title),
            "spec.md": ("spec.template.md", "[技术方案与契约设计]", f"{title} - 技术契约"),
            "plan.md": ("plan.template.md", "[实施与修改计划]", f"{title} - 实施计划"),
        }
        if doc_name not in tpl_map:
            return
        tpl_file, ph_title, rep_title = tpl_map[doc_name]
        tpl_path = self.template_dir / tpl_file
        if not tpl_path.exists():
            return

        content = (
            tpl_path.read_text(encoding="utf-8")
            .replace(ph_title, rep_title)
            .replace("TASK-XXX", task_id)
            .replace("[姓名/角色]", owner)
            .replace("[全栈开发工程师 / 架构师]", owner)
            .replace("[开发工程师 + 辅助 Agent]", owner)
            .replace("Tier 2 (Normal) / Tier 3 (High-risk)", risk_str)
            .replace("[YYYY-MM-DD]", created_at)
            .replace("[创建时间]", created_at)
        )
        target_file.write_text(content, encoding="utf-8")

    def create(
        self,
        task_id: str,
        title: str,
        tier: int = 2,
        owner: str = "Dev",
        stage: str = "Plan",
        next_action: str | None = None,
    ) -> None:
        tasks = self.parse_active_tasks()
        if task_id in tasks:
            raise ValueError(f"任务 '{task_id}' 已存在于活跃列表中！")

        if stage not in VALID_STAGES:
            raise ValueError(f"无效的 Stage: '{stage}'。可选值: {', '.join(VALID_STAGES)}")

        risk_str = f"Tier {tier}"
        if risk_str not in VALID_RISKS:
            raise ValueError(f"无效的 Tier: {tier}。可选值: 1, 2, 3")

        sdlc_path = f"docs/sdlc/{task_id}/" if tier in (2, 3) else "None"
        if next_action is None:
            next_action = "编写 intent.md 与初步方案" if tier in (2, 3) else "执行最小化代码实现与验证"

        created_at = datetime.datetime.now(datetime.timezone.utc).astimezone().strftime("%Y-%m-%d %H:%M")

        # Tier 2/3 按阶段物化工件 (默认 Plan 仅初始化 intent.md)
        if tier in (2, 3):
            self._materialize_doc(task_id, "intent.md", title, owner, risk_str, created_at=created_at)
            if stage in ("Design", "Build", "Test", "Review", "Deploy"):
                self._materialize_doc(task_id, "spec.md", title, owner, risk_str, created_at=created_at)
            if stage in ("Build", "Test", "Review", "Deploy"):
                self._materialize_doc(task_id, "plan.md", title, owner, risk_str, created_at=created_at)

        card = TaskCard(
            task_id=task_id,
            stage=stage,
            status="in_progress",
            risk=risk_str,
            owner=owner,
            created=created_at,
            next_action=next_action,
            blocked="None",
            sdlc_path=sdlc_path,
        )
        tasks[task_id] = card
        self._save_active_tasks(tasks)
        print(f"✅ 成功创建任务 [{task_id}] ({risk_str})")
        if tier in (2, 3):
            created_docs = ["intent.md"]
            if stage in ("Design", "Build", "Test", "Review", "Deploy"):
                created_docs.append("spec.md")
            if stage in ("Build", "Test", "Review", "Deploy"):
                created_docs.append("plan.md")
            print(f"📁 已初始化工件目录: docs/sdlc/{task_id}/ ({', '.join(created_docs)})")

    def update(
        self,
        task_id: str,
        stage: str | None = None,
        status: str | None = None,
        risk: str | None = None,
        owner: str | None = None,
        next_action: str | None = None,
        blocked: str | None = None,
    ) -> None:
        tasks = self.parse_active_tasks()
        if task_id not in tasks:
            raise KeyError(f"未找到活跃任务 '{task_id}'")

        card = tasks[task_id]

        if stage is not None:
            if stage not in VALID_STAGES:
                raise ValueError(f"无效的 Stage: '{stage}'。可选值: {', '.join(VALID_STAGES)}")
            old_stage = card.stage
            card.stage = stage

            # 若为 Tier 2 / 3，处理工件生命周期（打回安全备份、推进释放与备份恢复）
            if card.risk in ("Tier 2", "Tier 3"):
                artifact_dir = self.sdlc_dir / task_id
                old_idx = STAGE_ORDER.get(old_stage, 0)
                new_idx = STAGE_ORDER.get(stage, 0)

                # 场景 1: 阶段回退 (Rollback / 打回重做)，将超出目标阶段的工件备份为 .bak，解除 check 防偷跑自锁
                if new_idx < old_idx and artifact_dir.exists():
                    docs_to_backup = []
                    if stage == "Plan":
                        docs_to_backup = ["spec.md", "plan.md"]
                    elif stage == "Design":
                        docs_to_backup = ["plan.md"]

                    for doc in docs_to_backup:
                        doc_path = artifact_dir / doc
                        bak_path = artifact_dir / f"{doc}.bak"
                        if doc_path.exists():
                            if not bak_path.exists():
                                doc_path.rename(bak_path)
                            else:
                                doc_path.unlink()
                            print(f"📦 阶段回退 ({old_stage} -> {stage}): 已安全备份 {doc} 为 {doc}.bak，避免门禁死锁")

                # 场景 2: 向前推进或恢复 (Advance / Restore)
                title = task_id
                intent_file = self.sdlc_dir / task_id / "intent.md"
                if intent_file.exists():
                    first_line = intent_file.read_text(encoding="utf-8").splitlines()[0]
                    if "# Intent:" in first_line:
                        title = first_line.replace("# Intent:", "").strip()

                if stage in ("Design", "Build", "Test", "Review", "Deploy"):
                    spec_file = artifact_dir / "spec.md"
                    spec_bak = artifact_dir / "spec.md.bak"
                    if not spec_file.exists() and spec_bak.exists():
                        spec_bak.rename(spec_file)
                        print("♻️ 已从备份恢复工件: spec.md.bak -> spec.md")
                    else:
                        self._materialize_doc(task_id, "spec.md", title, card.owner, card.risk)

                if stage in ("Build", "Test", "Review", "Deploy"):
                    plan_file = artifact_dir / "plan.md"
                    plan_bak = artifact_dir / "plan.md.bak"
                    if not plan_file.exists() and plan_bak.exists():
                        plan_bak.rename(plan_file)
                        print("♻️ 已从备份恢复工件: plan.md.bak -> plan.md")
                    else:
                        self._materialize_doc(task_id, "plan.md", title, card.owner, card.risk)

        if status is not None:
            if status not in VALID_STATUSES:
                raise ValueError(f"无效的 Status: '{status}'。可选值: {', '.join(VALID_STATUSES)}")
            card.status = status

        if risk is not None:
            if risk not in VALID_RISKS:
                raise ValueError(f"无效的 Risk: '{risk}'。可选值: {', '.join(VALID_RISKS)}")
            card.risk = risk

        if owner is not None:
            card.owner = owner

        if next_action is not None:
            card.next_action = next_action

        if blocked is not None:
            card.blocked = blocked

        self._save_active_tasks(tasks)
        print(f"🔄 成功更新任务 [{task_id}]")

    def check(self, task_id: str | None = None) -> bool:
        tasks = self.parse_active_tasks()
        if not tasks:
            print("ℹ️ 当前暂无活跃任务。")
            return True

        targets = [tasks[task_id]] if task_id else list(tasks.values())
        all_passed = True

        for card in targets:
            print(f"\n🔍 检查任务: [{card.task_id}] (Stage: {card.stage}, Risk: {card.risk})")
            issues = []

            # 1. 基础状态校验
            if card.stage not in VALID_STAGES:
                issues.append(f"Stage '{card.stage}' 不合法 (可选: {VALID_STAGES})")
            if card.status not in VALID_STATUSES:
                issues.append(f"Status '{card.status}' 不合法 (可选: {VALID_STATUSES})")

            # 2. 工件存在性、防偷跑与完整性校验 (针对 Tier 2 / 3)
            if card.risk in ("Tier 2", "Tier 3"):
                artifact_dir = self.sdlc_dir / card.task_id
                if not artifact_dir.exists() or not artifact_dir.is_dir():
                    issues.append(f"工件目录缺失: docs/sdlc/{card.task_id}/")
                else:
                    # 防偷跑校验 (Anti-leapfrog check)
                    if card.stage == "Plan":
                        if (artifact_dir / "spec.md").exists():
                            issues.append("当前处于 Plan 阶段，严禁提前创建/修改 spec.md (需经用户审批 intent 并推进至 Design)")
                        if (artifact_dir / "plan.md").exists():
                            issues.append("当前处于 Plan 阶段，严禁提前创建/修改 plan.md (需经用户审批并推进至 Build)")
                        stage_docs = ["intent.md"]
                    elif card.stage == "Design":
                        if (artifact_dir / "plan.md").exists():
                            issues.append("当前处于 Design 阶段，严禁提前创建/修改 plan.md (需经用户审批 spec 并推进至 Build)")
                        stage_docs = ["intent.md", "spec.md"]
                    else:
                        stage_docs = ["intent.md", "spec.md", "plan.md"]

                    for doc_name in stage_docs:
                        doc_file = artifact_dir / doc_name
                        if not doc_file.exists():
                            issues.append(f"工件文件缺失: {doc_name}")
                        else:
                            content = doc_file.read_text(encoding="utf-8")
                            # 检查模板未替换的占位符（精准匹配模板提示语，防御普通正文中括号误报）
                            unfilled_patterns = [
                                r"\[(?:简明任务或缺陷标题|技术方案与契约设计|实施与修改计划)\]",
                                r"\[(?:客观描述当前存在的问题|清晰描述期望达到的具体业务成果|文字或\s*Mermaid|独立纯函数计算核|外部依赖与\s*Mock|评估过的替代方案|未采纳原因与权衡分析|回滚与故障应急策略|硬性技术制约|明确非目标|完成判定条件|未决疑问与待探讨点|操作目标|预期判据|核验结果|描述具体实施计划|无偏差\s*/\s*记录).*?\]",
                                r"\[(?:姓名/角色|创建时间|全栈开发工程师\s*/\s*架构师|开发工程师\s*\+\s*辅助\s*Agent|提出人或\s*Tech\s*Lead|技术负责人\s*/\s*架构评审人|执行工程师\s*/\s*辅助\s*Agent)\]",
                                r"\[YYYY-MM-DD\]",
                                r"\bTASK-XXX\b",
                                r"\[例如：(?:不可引入新框架依赖|方案\s*B\s*采用纯同步阻塞调用).*?\]",
                            ]
                            unfilled: list[str] = []
                            for pat in unfilled_patterns:
                                m = re.findall(pat, content)
                                if m:
                                    unfilled.extend(m)
                            if unfilled:
                                issues.append(f"{doc_name} 包含尚未填写的占位符: {unfilled[:2]}")

            # 3. 阻塞状态提醒
            if card.status == "blocked" and card.blocked == "None":
                issues.append("状态为 blocked，但未提供具体的 Blocked 原因说明")

            if issues:
                all_passed = False
                print("  ❌ 发现问题:")
                for issue in issues:
                    print(f"    - {issue}")
            else:
                print("  ✅ 门禁检查通过，工件完整规范。")

        return all_passed

    def archive(
        self,
        task_id: str,
        outcome: str,
        commit_pr: str,
        verification: str,
        final_stage: str = "Deploy",
        completed_date: str | None = None,
    ) -> None:
        tasks = self.parse_active_tasks()
        if task_id not in tasks:
            raise KeyError(f"未在活跃任务列表中找到 '{task_id}'！")

        self._ensure_archive_file()
        if completed_date is None:
            completed_date = datetime.datetime.now(datetime.timezone.utc).date().isoformat()

        # 追加到 ARCHIVE.md
        archive_content = self.archive_file.read_text(encoding="utf-8")
        row = f"| {task_id} | {outcome} | {final_stage} | `{commit_pr}` | {verification} | {completed_date} |\n"

        if row not in archive_content:
            archive_content = archive_content.rstrip() + "\n" + row
            self.archive_file.write_text(archive_content, encoding="utf-8")

        # 从 ACTIVE_TASKS.md 移除
        del tasks[task_id]
        self._save_active_tasks(tasks)
        print(f"📦 成功归档任务 [{task_id}]")
        print("📝 交付成果已沉淀至 docs/sdlc/ARCHIVE.md")

    def delete(self, task_id: str, force: bool = False) -> None:
        tasks = self.parse_active_tasks()
        if task_id not in tasks:
            raise KeyError(f"未在活跃任务列表中找到 '{task_id}'！")

        del tasks[task_id]
        self._save_active_tasks(tasks)

        artifact_dir = self.sdlc_dir / task_id
        if force and artifact_dir.exists():
            shutil.rmtree(artifact_dir)
            print(f"🗑️ 已删除活跃条目及工件目录: docs/sdlc/{task_id}/")
        else:
            print(f"🗑️ 已从活跃任务中注销 [{task_id}] (工件保留在 docs/sdlc/{task_id}/)")

    def list_tasks(self) -> None:
        tasks = self.parse_active_tasks()
        if not tasks:
            print("ℹ️ 当前没有活跃任务。")
            return

        stage_counts: dict[str, int] = {}
        for t in tasks.values():
            stage_counts[t.stage] = stage_counts.get(t.stage, 0) + 1
        stats_str = ", ".join(f"{s}: {c}" for s, c in sorted(stage_counts.items()))

        print(f"\n📋 当前活跃任务总览 ({len(tasks)} 个 | 阶段分布: {stats_str}):")
        print("-" * 75)
        for t in tasks.values():
            status_icon = "🚧" if t.status == "in_progress" else ("🚫" if t.status == "blocked" else "👀")
            created_str = f" | Created: {t.created}" if t.created else ""
            print(f"{status_icon} [{t.task_id}] ({t.risk}) | Stage: {t.stage} | Status: {t.status} | Owner: {t.owner}{created_str}")
            print(f"   Next: {t.next_action}")
            if t.blocked != "None":
                print(f"   Blocked: {t.blocked}")
        print("-" * 75)

    def show(self, task_id: str) -> None:
        tasks = self.parse_active_tasks()
        if task_id not in tasks:
            raise KeyError(f"未找到活跃任务 '{task_id}'")

        card = tasks[task_id]
        status_icon = "🚧" if card.status == "in_progress" else ("🚫" if card.status == "blocked" else "👀")
        print(f"\n{status_icon} 任务卡片快照: [{card.task_id}]")
        print("-" * 65)
        print(f"  • Stage:       {card.stage}")
        print(f"  • Status:      {card.status}")
        print(f"  • Risk:        {card.risk}")
        print(f"  • Owner:       {card.owner}")
        if card.created:
            print(f"  • Created:     {card.created}")
        print(f"  • Next Action: {card.next_action}")
        if card.blocked != "None":
            print(f"  • Blocked:     {card.blocked}")
        print(f"  • SDLC Dir:    {card.sdlc_path}")

        print("\n📄 工件生命周期状态:")
        artifact_dir = self.sdlc_dir / card.task_id
        if not artifact_dir.exists():
            print(f"  (工件目录尚未创建: {artifact_dir})")
        else:
            docs = ["intent.md", "spec.md", "plan.md"]
            for doc in docs:
                doc_path = artifact_dir / doc
                bak_path = artifact_dir / f"{doc}.bak"
                if doc_path.exists():
                    content = doc_path.read_text(encoding="utf-8")
                    lines_cnt = len(content.splitlines())
                    size_bytes = len(content.encode("utf-8"))
                    signoff_match = re.search(
                        r"\*\*准出结论\*\*:\s*([^\n]+)|\*\*审查结论\*\*:\s*([^\n]+)|\*\*验收结论\*\*:\s*([^\n]+)",
                        content,
                    )
                    signoff_str = ""
                    if signoff_match:
                        res = signoff_match.group(1) or signoff_match.group(2) or signoff_match.group(3)
                        signoff_str = f" | 结论: {res.strip()}"
                    print(f"  ✅ {doc:<10} (存在, {lines_cnt} 行, {size_bytes} 字节{signoff_str})")
                elif bak_path.exists():
                    print(f"  📦 {doc:<10} (已备份为 {doc}.bak - 阶段回退保护中)")
                else:
                    print(f"  ⚪ {doc:<10} (未释放/不存在)")
        print("-" * 65)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="task_cli",
        description="Xray Panel SDLC 任务生命周期管理工具 (task_cli)",
    )
    parser.add_argument("--root", type=str, default=None, help="指定项目根目录路径 (默认自动推导)")

    subparsers = parser.add_subparsers(dest="subcommand", required=True)

    # list
    subparsers.add_parser("list", help="列出当前所有活跃任务")

    # show
    show_parser = subparsers.add_parser("show", help="展示指定任务的卡片全貌与工件生命周期快照")
    show_parser.add_argument("task_id", help="任务标识")

    # create
    create_parser = subparsers.add_parser("create", help="创建新任务并初始化对应工件与卡片")
    create_parser.add_argument("task_id", help="任务标识，如 TASK-001 或 core-rubric-scorer")
    create_parser.add_argument("--title", required=True, help="简明任务标题或缺陷摘要")
    create_parser.add_argument("--tier", type=int, choices=[1, 2, 3], default=2, help="风险分级 (1: 轻量, 2: 常规, 3: 高危)")
    create_parser.add_argument("--owner", default="Dev", help="任务负责人/角色 (默认: Dev)")
    create_parser.add_argument("--stage", default="Plan", choices=VALID_STAGES, help="初始阶段 (默认: Plan)")
    create_parser.add_argument("--next", dest="next_action", default=None, help="下一步具体动作")

    # update
    update_parser = subparsers.add_parser("update", help="更新任务阶段、状态或卡片信息")
    update_parser.add_argument("task_id", help="任务标识")
    update_parser.add_argument("--stage", choices=VALID_STAGES, help="变更阶段")
    update_parser.add_argument("--status", choices=VALID_STATUSES, help="变更状态")
    update_parser.add_argument("--risk", choices=VALID_RISKS, help="变更风险等级")
    update_parser.add_argument("--owner", help="变更负责人")
    update_parser.add_argument("--next", dest="next_action", help="更新下一步动作")
    update_parser.add_argument("--blocked", help="记录或解除阻塞原因 ('None' 表示解除)")

    # check
    check_parser = subparsers.add_parser("check", help="执行门禁检查与工件完整性校验")
    check_parser.add_argument("task_id", nargs="?", default=None, help="待检查的任务标识 (可选，默认检查全部)")

    # archive
    archive_parser = subparsers.add_parser("archive", help="完成验收并归档任务，沉淀成果到 ARCHIVE.md")
    archive_parser.add_argument("task_id", help="任务标识")
    archive_parser.add_argument("--outcome", required=True, help="交付成果摘要")
    archive_parser.add_argument("--commit", dest="commit_pr", required=True, help="关联 Commit SHA 或 PR 链接")
    archive_parser.add_argument("--verify", required=True, help="验证结果或测试通过情况 (如: 42 tests passed)")
    archive_parser.add_argument("--final-stage", default="Deploy", choices=VALID_STAGES, help="最终阶段 (默认: Deploy)")
    archive_parser.add_argument("--date", default=None, help="完成日期 (YYYY-MM-DD，默认今天)")

    # delete
    del_parser = subparsers.add_parser("delete", help="注销或删除任务")
    del_parser.add_argument("task_id", help="任务标识")
    del_parser.add_argument("--force", action="store_true", help="是否同时强行删除 docs/sdlc/<task-id>/ 目录")

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()

    root_path = Path(args.root) if args.root else None
    manager = TaskManager(workspace_root=root_path)

    try:
        if args.subcommand == "list":
            manager.list_tasks()
        elif args.subcommand == "show":
            manager.show(task_id=args.task_id)
        elif args.subcommand == "create":
            manager.create(
                task_id=args.task_id,
                title=args.title,
                tier=args.tier,
                owner=args.owner,
                stage=args.stage,
                next_action=args.next_action,
            )
        elif args.subcommand == "update":
            manager.update(
                task_id=args.task_id,
                stage=args.stage,
                status=args.status,
                risk=args.risk,
                owner=args.owner,
                next_action=args.next_action,
                blocked=args.blocked,
            )
        elif args.subcommand == "check":
            passed = manager.check(args.task_id)
            return 0 if passed else 1
        elif args.subcommand == "archive":
            manager.archive(
                task_id=args.task_id,
                outcome=args.outcome,
                commit_pr=args.commit_pr,
                verification=args.verify,
                final_stage=args.final_stage,
                completed_date=args.date,
            )
        elif args.subcommand == "delete":
            manager.delete(task_id=args.task_id, force=args.force)
        return 0
    except Exception as e:  # noqa: BLE001
        print(f"❌ 执行失败: {e}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
