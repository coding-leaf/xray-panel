#!/usr/bin/env python3
"""OpenCode Subagent Token & Cache Statistics Viewer.

Inspects the OpenCode SQLite database to display token usage, reasoning tokens,
prompt cache hit rates, and costs for subagents of recent parent sessions.
Zero external dependencies (pure standard library).
"""

from __future__ import annotations

import argparse
import datetime
import json
import os
import sqlite3
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple


def _default_db_path() -> Path:
    env_path = os.environ.get("OPENCODE_DB_PATH")
    if env_path:
        return Path(env_path).expanduser()
    return Path("~/.local/share/opencode/opencode.db").expanduser()


# ANSI 颜色定义
USE_COLOR = sys.stdout.isatty()


def _color(text: str, code: str) -> str:
    if not USE_COLOR:
        return text
    return f"\033[{code}m{text}\033[0m"


def bold(text: str) -> str:
    return _color(text, "1")


def dim(text: str) -> str:
    return _color(text, "2")


def green(text: str) -> str:
    return _color(text, "32")


def cyan(text: str) -> str:
    return _color(text, "36")


def blue(text: str) -> str:
    return _color(text, "34")


def yellow(text: str) -> str:
    return _color(text, "33")


def magenta(text: str) -> str:
    return _color(text, "35")


def format_num(val: Optional[int]) -> str:
    if val is None or val == 0:
        return "0"
    if val >= 1_000_000:
        return f"{val:,} ({val / 1_000_000:.2f}M)"
    if val >= 10_000:
        return f"{val:,} ({val / 1_000:.1f}k)"
    return f"{val:,}"


def format_compact_num(val: Optional[int]) -> str:
    if val is None or val == 0:
        return "0"
    return f"{val:,}"


def format_rate(read: int, inp: int) -> str:
    total = read + inp
    if total <= 0:
        return "0.0%"
    rate = (read / total) * 100.0
    s = f"{rate:5.1f}%"
    if rate >= 80:
        return green(s)
    if rate >= 50:
        return yellow(s)
    return dim(s)


def format_model_name(raw_model: Optional[str]) -> str:
    if not raw_model:
        return "-"
    try:
        data = json.loads(raw_model)
        if isinstance(data, dict) and "id" in data:
            return str(data["id"])
    except Exception:
        pass
    return str(raw_model)


def format_duration(start_ms: Optional[int], end_ms: Optional[int]) -> str:
    if not start_ms or not end_ms or end_ms <= start_ms:
        return "-"
    sec = (end_ms - start_ms) / 1000.0
    if sec < 60:
        return f"{sec:.1f}s"
    return f"{sec / 60:.1f}m"


def fetch_parent_sessions(
    conn: sqlite3.Connection,
    limit: int = 1,
    repo_dir: Optional[str] = None,
    subagents_only: bool = False,
) -> List[str]:
    """获取最近的主任务会话 ID 列表。优先匹配当前工作区目录。"""
    conditions = ["(parent_id IS NULL OR parent_id = '')"]
    params: List[Any] = []

    if repo_dir:
        conditions.append("(directory = ? OR directory LIKE ?)")
        params.extend([repo_dir, f"{repo_dir}/%"])

    if subagents_only:
        conditions.append(
            "id IN (SELECT DISTINCT parent_id FROM session WHERE parent_id IS NOT NULL AND parent_id != '')"
        )

    where_clause = " AND ".join(conditions)
    query = f"""
        SELECT id
        FROM session
        WHERE {where_clause}
        ORDER BY time_updated DESC
        LIMIT ?
    """
    params.append(limit)
    rows = conn.execute(query, tuple(params)).fetchall()

    # 如果指定当前目录无结果，回退到全局不限目录查询
    if not rows and repo_dir:
        return fetch_parent_sessions(
            conn, limit=limit, repo_dir=None, subagents_only=subagents_only
        )

    return [r[0] for r in rows]


def get_session_info(conn: sqlite3.Connection, session_id: str) -> Optional[Dict[str, Any]]:
    conn.row_factory = sqlite3.Row
    row = conn.execute("SELECT * FROM session WHERE id = ?", (session_id,)).fetchone()
    if not row:
        return None
    return dict(row)


def get_subagents_for_parent(conn: sqlite3.Connection, parent_id: str) -> List[Dict[str, Any]]:
    conn.row_factory = sqlite3.Row
    query = """
        SELECT * FROM session
        WHERE parent_id = ?
        ORDER BY time_created ASC
    """
    rows = conn.execute(query, (parent_id,)).fetchall()
    return [dict(r) for r in rows]


def render_table(
    headers: List[str],
    rows: List[List[str]],
    alignments: List[str],  # 'left' or 'right'
    total_row: Optional[List[str]] = None,
) -> str:
    """纯文本 / ANSI 对齐美化表格渲染器。"""
    col_count = len(headers)
    
    # 计算无颜色控制字符时的视觉宽度
    import re
    ansi_escape = re.compile(r"\x1B(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])")

    def visual_len(s: str) -> int:
        return len(ansi_escape.sub("", s))

    col_widths = [visual_len(h) for h in headers]
    for r in rows:
        for i, val in enumerate(r):
            col_widths[i] = max(col_widths[i], visual_len(val))
    if total_row:
        for i, val in enumerate(total_row):
            col_widths[i] = max(col_widths[i], visual_len(val))

    # 边框字符
    # ┌─┬─┐ │ ├─┼─┤ └─┴─┘
    top_line = "┌" + "┬".join("─" * (w + 2) for w in col_widths) + "┐"
    header_sep = "├" + "┼".join("─" * (w + 2) for w in col_widths) + "┤"
    bottom_line = "└" + "┴".join("─" * (w + 2) for w in col_widths) + "┘"

    lines = [top_line]

    # Header
    header_cells = []
    for i, h in enumerate(headers):
        w = col_widths[i]
        pad = w - visual_len(h)
        header_cells.append(f" {bold(h)}{' ' * pad} ")
    lines.append("│" + "│".join(header_cells) + "│")
    lines.append(header_sep)

    # Data Rows
    for r in rows:
        cells = []
        for i, val in enumerate(r):
            w = col_widths[i]
            pad = w - visual_len(val)
            if alignments[i] == "right":
                cells.append(f" {' ' * pad}{val} ")
            else:
                cells.append(f" {val}{' ' * pad} ")
        lines.append("│" + "│".join(cells) + "│")

    # Total Row
    if total_row:
        lines.append(header_sep)
        cells = []
        for i, val in enumerate(total_row):
            w = col_widths[i]
            pad = w - visual_len(val)
            if alignments[i] == "right":
                cells.append(f" {' ' * pad}{val} ")
            else:
                cells.append(f" {val}{' ' * pad} ")
        lines.append("│" + "│".join(cells) + "│")

    lines.append(bottom_line)
    return "\n".join(lines)


def print_session_report(
    conn: sqlite3.Connection, parent_id: str, subagents_only: bool = False
) -> None:
    parent = get_session_info(conn, parent_id)
    subs = get_subagents_for_parent(conn, parent_id)

    title = parent.get("title") if parent else "(Unknown Session)"
    created_at = ""
    if parent and parent.get("time_created"):
        created_at = datetime.datetime.fromtimestamp(
            parent["time_created"] / 1000
        ).strftime("%Y-%m-%d %H:%M:%S")

    sub_count = len(subs)
    sub_note = dim("(纯主会话直通执行)") if sub_count == 0 else ""

    print()
    print(bold(cyan("🎯 主任务会话: ")) + bold(str(title)))
    print(
        dim(
            f"   Session ID: {parent_id}  |  创建时间: {created_at}  |  子代理调用数: {sub_count} {sub_note}".strip()
        )
    )
    print()

    if subagents_only and not subs:
        print(yellow("   未检测到该会话派生出的子代理。"))
        return

    headers = [
        "Agent",
        "Model",
        "Input (Pmt)",
        "Cache Read",
        "Cache Hit %",
        "Output (Cpl)",
        "Reasoning",
        "Duration",
    ]
    alignments = ["left", "left", "right", "right", "right", "right", "right", "right"]

    rows = []
    tot_input = 0
    tot_cache_read = 0
    tot_output = 0
    tot_reasoning = 0

    # 1. 主会话自身消耗 (若非 subagents_only 模式)
    parent_has_tokens = False
    parent_inp = 0
    parent_cread = 0
    parent_outp = 0
    parent_reasoning = 0

    if not subagents_only and parent:
        parent_model = format_model_name(parent.get("model"))
        parent_inp = parent.get("tokens_input") or 0
        parent_cread = parent.get("tokens_cache_read") or 0
        parent_outp = parent.get("tokens_output") or 0
        parent_reasoning = parent.get("tokens_reasoning") or 0
        parent_duration = format_duration(
            parent.get("time_created"), parent.get("time_updated")
        )

        tot_input += parent_inp
        tot_cache_read += parent_cread
        tot_output += parent_outp
        tot_reasoning += parent_reasoning
        parent_has_tokens = True

        rows.append([
            bold(blue("main (lead)")),
            dim(parent_model),
            format_compact_num(parent_inp),
            format_compact_num(parent_cread),
            format_rate(parent_cread, parent_inp),
            format_compact_num(parent_outp),
            format_compact_num(parent_reasoning) if parent_reasoning > 0 else "-",
            dim(parent_duration),
        ])

    # 2. 各子代理消耗
    sub_inp_tot = 0
    sub_cread_tot = 0
    sub_outp_tot = 0
    sub_reasoning_tot = 0

    for s in subs:
        agent_name = s.get("agent") or "subagent"
        model_name = format_model_name(s.get("model"))
        inp = s.get("tokens_input") or 0
        cread = s.get("tokens_cache_read") or 0
        outp = s.get("tokens_output") or 0
        reasoning = s.get("tokens_reasoning") or 0
        duration = format_duration(s.get("time_created"), s.get("time_updated"))

        tot_input += inp
        tot_cache_read += cread
        tot_output += outp
        tot_reasoning += reasoning

        sub_inp_tot += inp
        sub_cread_tot += cread
        sub_outp_tot += outp
        sub_reasoning_tot += reasoning

        # Agent 彩色标记
        if agent_name == "builder":
            agent_colored = green("builder")
        elif agent_name == "planner":
            agent_colored = cyan("planner")
        elif agent_name == "reviewer":
            agent_colored = magenta("reviewer")
        else:
            agent_colored = yellow(agent_name)

        rows.append([
            agent_colored,
            dim(model_name),
            format_compact_num(inp),
            format_compact_num(cread),
            format_rate(cread, inp),
            format_compact_num(outp),
            format_compact_num(reasoning) if reasoning > 0 else "-",
            dim(duration),
        ])

    # 汇总标签
    if parent_has_tokens and subs:
        summary_label = dim(f"main + {len(subs)} subs")
    elif parent_has_tokens:
        summary_label = dim("main only")
    else:
        summary_label = dim(f"{len(subs)} subagents")

    total_row = [
        bold("TOTAL"),
        summary_label,
        bold(format_compact_num(tot_input)),
        bold(green(format_compact_num(tot_cache_read))),
        bold(format_rate(tot_cache_read, tot_input)),
        bold(format_compact_num(tot_output)),
        bold(format_compact_num(tot_reasoning)) if tot_reasoning > 0 else "-",
        "-",
    ]

    print(render_table(headers, rows, alignments, total_row=total_row))

    # 关键指标高亮摘要
    total_prompt = tot_input + tot_cache_read
    global_hit_rate = (tot_cache_read / total_prompt * 100.0) if total_prompt > 0 else 0.0
    print()
    print(
        f"  📊 {bold('总 Prompt Tokens')}: {format_num(total_prompt)} "
        f"│ {bold('缓存命中 (Cache Read)')}: {green(format_num(tot_cache_read))} "
        f"│ {bold('整体命中率')}: {green(f'{global_hit_rate:.1f}%')}"
    )
    print(
        f"  📤 {bold('总 Completion Tokens')}: {format_num(tot_output)} "
        f"│ {bold('思考链 (Reasoning)')}: {format_num(tot_reasoning)}"
    )

    # 若同时存在主会话与子代理，补充结构拆解
    if parent_has_tokens and subs:
        main_prompt = parent_inp + parent_cread
        main_rate = (parent_cread / main_prompt * 100.0) if main_prompt > 0 else 0.0
        sub_prompt = sub_inp_tot + sub_cread_tot
        sub_rate = (sub_cread_tot / sub_prompt * 100.0) if sub_prompt > 0 else 0.0

        print(
            f"  🔹 {dim('主会话开销')}: Prompt {format_num(main_prompt)} (Cache {format_num(parent_cread)}, {main_rate:.1f}%) "
            f"│ 输出 {format_num(parent_outp)} │ 思考链 {format_num(parent_reasoning)}"
        )
        print(
            f"  🔸 {dim(f'子代理开销 ({len(subs)}个)')}: Prompt {format_num(sub_prompt)} (Cache {format_num(sub_cread_tot)}, {sub_rate:.1f}%) "
            f"│ 输出 {format_num(sub_outp_tot)} │ 思考链 {format_num(sub_reasoning_tot)}"
        )

    print()


def main() -> int:
    parser = argparse.ArgumentParser(
        description="OpenCode 会话与子代理 Token 及 Prompt Cache 统计工具"
    )
    parser.add_argument(
        "-n",
        "--sessions",
        type=int,
        default=1,
        help="查看最近几个主任务会话（默认: 1，即当前/最近的主任务）",
    )
    parser.add_argument(
        "-s",
        "--session-id",
        type=str,
        default=None,
        help="指定特定的父会话 ID（可选）",
    )
    parser.add_argument(
        "-S",
        "--subagents-only",
        action="store_true",
        help="仅统计子代理，过滤无子代理的历史会话并排除主会话行",
    )
    parser.add_argument(
        "--all-dirs",
        action="store_true",
        help="不限制当前工程目录，跨项目搜索全局最近会话",
    )
    parser.add_argument(
        "--db-path",
        type=str,
        default=None,
        help="OpenCode SQLite 数据库绝对路径（默认自动探测）",
    )

    args = parser.parse_args()

    db_file = Path(args.db_path).expanduser() if args.db_path else _default_db_path()
    if not db_file.exists():
        sys.stderr.write(f"❌ 找不到 OpenCode 数据库文件: {db_file}\n")
        return 1

    try:
        conn = sqlite3.connect(str(db_file))
    except Exception as e:
        sys.stderr.write(f"❌ 连接 SQLite 失败: {e}\n")
        return 1

    try:
        if args.session_id:
            parent_ids = [args.session_id]
        else:
            repo_dir = None if args.all_dirs else os.getcwd()
            parent_ids = fetch_parent_sessions(
                conn,
                limit=args.sessions,
                repo_dir=repo_dir,
                subagents_only=args.subagents_only,
            )

        if not parent_ids:
            print(yellow("💡 未找到匹配的会话记录。"))
            return 0

        for pid in parent_ids:
            print_session_report(conn, pid, subagents_only=args.subagents_only)

    finally:
        conn.close()

    return 0


if __name__ == "__main__":
    sys.exit(main())
