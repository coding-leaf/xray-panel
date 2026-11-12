#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Universal Trellis Lifecycle Hook for Google Antigravity (AGY 2.0).

Bridges Trellis's spec/task/workflow system with Antigravity's native hooks.json:
  1. session-start   -> Injects compact Trellis session state & spec indexes once at startup.
  2. pre-invocation  -> Injects deduplicated <workflow-state> breadcrumb on new user turns
                        or task-status transitions (skipping Swarm workers/subagents).
  3. pre-tool-use    ->
       - Bridges `conversationId` into `task.py create/start/current/finish`.
       - Auto-injects curated `implement.jsonl` / `check.jsonl` spec manifests and
         task artifacts (`prd.md`, `design.md`, `implement.md`) into AGY `swarm`
         workers/verifiers and `invoke_subagent` calls via `overwrite`.

Portable & 100% project-agnostic: contains zero project-specific paths or commands.
"""
from __future__ import annotations

import warnings

warnings.filterwarnings("ignore")

import hashlib
import json
import os
import re
import shlex
import subprocess
import sys
import time
from pathlib import Path
from typing import Any

# Force UTF-8 streams on Windows so Chinese prompts/PRDs never raise codec errors.
if sys.platform.startswith("win"):
    import io as _io

    for _sname in ("stdin", "stdout", "stderr"):
        _stream = getattr(sys, _sname, None)
        if _stream is None:
            continue
        if hasattr(_stream, "reconfigure"):
            try:
                _stream.reconfigure(encoding="utf-8", errors="replace")
            except Exception:
                pass
        elif hasattr(_stream, "detach"):
            try:
                setattr(
                    sys,
                    _sname,
                    _io.TextIOWrapper(_stream.detach(), encoding="utf-8", errors="replace"),
                )
            except Exception:
                pass

DIR_WORKFLOW = ".trellis"
DIR_RUNTIME = ".runtime"
DIR_SHELL_TICKETS = "shell-tickets"
DIR_AGY_STATE = "agy-hook-state"
INJECT_MARKER = "<!-- trellis-swarm-injected -->"
TICKET_TTL_SECONDS = 30
COORDINATOR_TTL_SECONDS = 7200  # 2 hours

_TAG_RE = re.compile(
    r"\[workflow-state:([A-Za-z0-9_-]+)\]\s*\n(.*?)\n\s*\[/workflow-state:\1\]",
    re.DOTALL,
)
_REVIEWER_KEYWORDS = ("verifier", "review", "check", "qa", "audit")


# =============================================================================
# Basic Helpers
# =============================================================================


def _str(val: Any) -> str:
    return val.strip() if isinstance(val, str) else ""


def _read_stdin_json() -> dict[str, Any]:
    try:
        raw = sys.stdin.read()
        if not raw or not raw.strip():
            return {}
        data = json.loads(raw)
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


def _find_trellis_root(start: Path) -> Path | None:
    try:
        cur = start.resolve()
    except OSError:
        return None
    while True:
        if (cur / DIR_WORKFLOW).is_dir():
            return cur
        if cur == cur.parent:
            return None
        cur = cur.parent


def _resolve_repo_root(data: dict[str, Any]) -> Path | None:
    candidates: list[Path] = []
    ws_paths = data.get("workspacePaths")
    if isinstance(ws_paths, list):
        for p in ws_paths:
            if isinstance(p, str) and p.strip():
                candidates.append(Path(p.strip()))
    cwd_val = _str(data.get("cwd"))
    if cwd_val:
        candidates.append(Path(cwd_val))
    try:
        candidates.append(Path.cwd())
    except OSError:
        pass
    # Script itself lives at <repo>/.agent/hooks/trellis_agy_hook.py
    candidates.append(Path(__file__).resolve().parent)

    for candidate in candidates:
        root = _find_trellis_root(candidate)
        if root is not None:
            return root
    return None


def _ensure_trellis_imports(repo_root: Path) -> None:
    scripts_dir = repo_root / DIR_WORKFLOW / "scripts"
    if str(scripts_dir) not in sys.path:
        sys.path.insert(0, str(scripts_dir))


def _agy_state_dir(repo_root: Path) -> Path:
    d = repo_root / DIR_WORKFLOW / DIR_RUNTIME / DIR_AGY_STATE
    d.mkdir(parents=True, exist_ok=True)
    return d


def _read_json_file(path: Path) -> dict[str, Any]:
    try:
        if path.is_file():
            obj = json.loads(path.read_text(encoding="utf-8"))
            if isinstance(obj, dict):
                return obj
    except Exception:
        pass
    return {}


def _write_json_file(path: Path, payload: dict[str, Any]) -> None:
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        tmp = path.with_suffix(".tmp")
        tmp.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        tmp.replace(path)
    except Exception:
        pass


# =============================================================================
# Worker / Subagent Detection (Prevent Main-Session Prompts in Child Agents)
# =============================================================================


def _record_coordinator(repo_root: Path, conversation_id: str) -> None:
    if not conversation_id:
        return
    _write_json_file(
        _agy_state_dir(repo_root) / "swarm_coordinator.json",
        {"coordinator_id": conversation_id, "updated_at": time.time()},
    )


def _is_git_worktree(repo_root: Path) -> bool:
    """In `workspace:"share"` worktrees, `.git` is a file pointing to gitdir, not a directory."""
    git_entry = repo_root / ".git"
    return git_entry.is_file()


def _is_child_worker_session(
    repo_root: Path,
    data: dict[str, Any],
    has_own_session_pointer: bool,
) -> bool:
    # 1. `workspace:"share"` worktree is always a child worker.
    if _is_git_worktree(repo_root):
        return True

    # 2. If this conversation explicitly owns an active Trellis task pointer, it IS a main session.
    if has_own_session_pointer:
        return False

    # 3. Inspect lastUserInput for Swarm board addressing / injected brief markers.
    last_input = _str(data.get("lastUserInput"))
    if last_input:
        if (
            last_input.startswith("@")
            or INJECT_MARKER in last_input
            or "SWARM_DIR=" in last_input
            or "Load the swarm_monitor skill" in last_input
            or "Load the swarm_narrator skill" in last_input
        ):
            return True

    # 4. If a Coordinator recently spawned Swarm/Subagent workers, any other conversationId
    #    without its own Trellis task pointer is treated as a worker/subagent.
    conv_id = _str(data.get("conversationId"))
    coord = _read_json_file(_agy_state_dir(repo_root) / "swarm_coordinator.json")
    coord_id = _str(coord.get("coordinator_id"))
    updated_at = coord.get("updated_at")
    if (
        conv_id
        and coord_id
        and conv_id != coord_id
        and isinstance(updated_at, (int, float))
        and (time.time() - updated_at) < COORDINATOR_TTL_SECONDS
    ):
        return True

    return False


# =============================================================================
# 1. SessionStart Handler
# =============================================================================


def _run_git(repo_root: Path, args: list[str]) -> str:
    try:
        res = subprocess.run(
            ["git", *args],
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=3,
            cwd=str(repo_root),
        )
        return res.stdout.strip() if res.returncode == 0 else ""
    except Exception:
        return ""


def _collect_spec_indexes(repo_root: Path) -> list[str]:
    spec_root = repo_root / DIR_WORKFLOW / "spec"
    if not spec_root.is_dir():
        return []
    found: list[str] = []
    guides = spec_root / "guides" / "index.md"
    if guides.is_file():
        found.append(".trellis/spec/guides/index.md")
    for sub in sorted(spec_root.iterdir()):
        if not sub.is_dir() or sub.name.startswith(".") or sub.name == "guides":
            continue
        idx = sub / "index.md"
        if idx.is_file():
            found.append(f".trellis/spec/{sub.name}/index.md")
            continue
        for nested in sorted(sub.iterdir()):
            if nested.is_dir() and (nested / "index.md").is_file():
                found.append(f".trellis/spec/{sub.name}/{nested.name}/index.md")
    return found


def handle_session_start(repo_root: Path, data: dict[str, Any]) -> dict[str, Any]:
    _ensure_trellis_imports(repo_root)
    from common.active_task import resolve_active_task  # type: ignore[import-not-found]
    from common.paths import get_developer  # type: ignore[import-not-found]
    from common.workflow_phase import filter_platform, get_phase_index  # type: ignore[import-not-found]

    active = resolve_active_task(repo_root, data, platform="antigravity")
    has_own_pointer = bool(active.task_path and active.source_type == "session")
    if _is_child_worker_session(repo_root, data, has_own_pointer):
        return {}

    dev = get_developer(repo_root) or "(not initialized)"
    branch = _run_git(repo_root, ["branch", "--show-current"]) or "(detached)"
    dirty = [ln for ln in _run_git(repo_root, ["status", "--porcelain"]).splitlines() if ln.strip()]
    dirty_str = "clean" if not dirty else f"dirty ({len(dirty)} paths)"

    if active.task_path and not active.stale:
        t_json = _read_json_file(repo_root / active.task_path / "task.json")
        t_status = _str(t_json.get("status")) or "unknown"
        task_line = f"{active.task_path} (status={t_status})"
    elif active.task_path and active.stale:
        task_line = f"{active.task_path} (STALE — run `python ./.trellis/scripts/task.py finish`)"
    else:
        task_line = "none"

    spec_indexes = _collect_spec_indexes(repo_root)
    spec_block = "\n".join(f"  - {p}" for p in spec_indexes) if spec_indexes else "  (none)"

    phase_index = ""
    try:
        phase_index = filter_platform(get_phase_index(), "Antigravity").strip()
    except Exception:
        pass

    msg_parts = [
        "<trellis-session-context>",
        f"Developer: {dev} | Git: {branch} ({dirty_str}) | Active task: {task_line}",
        "Spec indexes (read relevant index before coding/planning):",
        spec_block,
    ]
    if phase_index:
        msg_parts.extend(["", phase_index])
    msg_parts.append("</trellis-session-context>")

    return {
        "injectSteps": [
            {
                "systemMessage": {
                    "systemMessage": "\n".join(msg_parts),
                }
            }
        ]
    }


# =============================================================================
# 2. PreInvocation Handler (Deduplicated Per-Turn <workflow-state> Breadcrumb)
# =============================================================================


def _load_breadcrumbs(repo_root: Path) -> dict[str, str]:
    wf = repo_root / DIR_WORKFLOW / "workflow.md"
    if not wf.is_file():
        return {}
    try:
        content = wf.read_text(encoding="utf-8")
    except OSError:
        return {}
    out: dict[str, str] = {}
    for m in _TAG_RE.finditer(content):
        status, body = m.group(1), m.group(2).strip()
        if body:
            out[status] = body
    return out


def _prompt_has_skip_keyword(prompt: str, repo_root: Path) -> bool:
    if not prompt:
        return False
    keyword = "no-trellis"
    try:
        _ensure_trellis_imports(repo_root)
        from common.trellis_config import read_trellis_config  # type: ignore[import-not-found]

        cfg = read_trellis_config(repo_root)
        if isinstance(cfg, dict) and isinstance(cfg.get("prompt_injection"), dict):
            raw = cfg["prompt_injection"].get("skip_keyword", keyword)
            if isinstance(raw, str):
                keyword = raw
    except Exception:
        pass
    if not keyword:
        return False
    pattern = r"(?<![\w-])" + re.escape(keyword) + r"(?![\w-])"
    return re.search(pattern, prompt, re.IGNORECASE) is not None


def handle_pre_invocation(repo_root: Path, data: dict[str, Any]) -> dict[str, Any]:
    _ensure_trellis_imports(repo_root)
    from common.active_task import resolve_active_task  # type: ignore[import-not-found]

    last_input = _str(data.get("lastUserInput"))
    if _prompt_has_skip_keyword(last_input, repo_root):
        return {}

    active = resolve_active_task(repo_root, data, platform="antigravity")
    has_own_pointer = bool(active.task_path and active.source_type == "session")
    if _is_child_worker_session(repo_root, data, has_own_pointer):
        return {}

    task_id: str | None = None
    status = "no_task"
    if active.task_path:
        t_dir = repo_root / active.task_path
        if active.stale or not t_dir.is_dir():
            task_id = t_dir.name
            status = "task_error"
        else:
            t_json = _read_json_file(t_dir / "task.json")
            task_id = _str(t_json.get("id")) or t_dir.name
            status = _str(t_json.get("status")) or "task_error"

    # Deduplicate across multi-step tool loops within the same user turn unless task status changed.
    conv_id = _str(data.get("conversationId")) or "default"
    safe_conv = re.sub(r"[^A-Za-z0-9._-]+", "_", conv_id)[:100]
    state_file = _agy_state_dir(repo_root) / f"inv_{safe_conv}.json"
    input_hash = hashlib.sha256(last_input.encode("utf-8")).hexdigest()[:16]
    status_sig = f"{task_id or ''}:{status}"

    prev = _read_json_file(state_file)
    if prev.get("input_hash") == input_hash and prev.get("status_sig") == status_sig:
        return {}

    _write_json_file(
        state_file,
        {"input_hash": input_hash, "status_sig": status_sig, "updated_at": time.time()},
    )

    templates = _load_breadcrumbs(repo_root)
    # Prefer Antigravity-specific state block (`<status>-antigravity`) before generic `<status>`.
    body = templates.get(f"{status}-antigravity") or templates.get(status)
    if not body:
        body = "Refer to .trellis/workflow.md for current step."

    header = f"Status: {status}" if task_id is None else f"Task: {task_id} ({status})"
    breadcrumb = f"<workflow-state>\n{header}\n{body}\n</workflow-state>"

    return {
        "injectSteps": [
            {
                "systemMessage": {
                    "systemMessage": breadcrumb,
                }
            }
        ]
    }


# =============================================================================
# 3. PreToolUse Handler (task.py Session Bridge + Swarm/Subagent Spec Injection)
# =============================================================================


def _extract_task_py_info(command: str) -> tuple[bool, bool, list[dict[str, str]]]:
    """Return (mentions_task_py, has_create, ticket_subcommands)."""
    if "task.py" not in command:
        return False, False, []
    try:
        tokens = shlex.split(command, posix=os.name != "nt")
    except ValueError:
        tokens = command.split()

    mentions = False
    has_create = False
    subcommands: list[dict[str, str]] = []
    for idx, tok in enumerate(tokens[:-1]):
        clean = tok.strip("\"'")
        if Path(clean).name != "task.py":
            continue
        mentions = True
        sub = tokens[idx + 1].strip("\"'")
        if sub == "create":
            has_create = True
        elif sub in ("start", "current", "finish"):
            item: dict[str, str] = {"name": sub}
            if sub == "start" and idx + 2 < len(tokens):
                item["task_ref"] = tokens[idx + 2].strip("\"'")
            subcommands.append(item)
    return mentions, has_create, subcommands


def _write_shell_ticket(
    repo_root: Path,
    data: dict[str, Any],
    context_key: str,
    command: str,
    subcommands: list[dict[str, str]],
) -> None:
    now = time.time()
    ticket_dir = repo_root / DIR_WORKFLOW / DIR_RUNTIME / DIR_SHELL_TICKETS
    ticket_dir.mkdir(parents=True, exist_ok=True)
    for old in ticket_dir.glob("*.json"):
        try:
            exp = _read_json_file(old).get("expires_at_epoch")
            if isinstance(exp, (int, float)) and exp < now:
                old.unlink()
        except OSError:
            pass

    digest = hashlib.sha256(f"{context_key}\0{command}\0{now}".encode("utf-8")).hexdigest()[:16]
    ticket_path = ticket_dir / f"{int(now * 1000)}-{digest}.json"
    payload = {
        "platform": "antigravity",
        "context_key": context_key,
        "conversation_id": _str(data.get("conversationId")) or None,
        "cwd": str(repo_root),
        "command": command,
        "subcommands": subcommands,
        "created_at_epoch": now,
        "expires_at_epoch": now + TICKET_TTL_SECONDS,
    }
    _write_json_file(ticket_path, payload)


def _read_curated_jsonl(task_dir_abs: Path, jsonl_name: str) -> list[tuple[str, str]]:
    path = task_dir_abs / jsonl_name
    if not path.is_file():
        return []
    entries: list[tuple[str, str]] = []
    try:
        for raw in path.read_text(encoding="utf-8").splitlines():
            line = raw.strip()
            if not line:
                continue
            try:
                item = json.loads(line)
            except json.JSONDecodeError:
                continue
            if not isinstance(item, dict):
                continue
            f = _str(item.get("file") or item.get("path"))
            if f:
                entries.append((f, _str(item.get("reason")) or "required spec/context"))
    except OSError:
        pass
    return entries


def _to_manifest_path(repo_root: Path, raw_path: str) -> str:
    """Resolve repo-relative task/spec paths to absolute POSIX paths so
    `workspace:"share"` / `"branch"` git worktrees can read untracked task
    artifacts and main-workspace specs without `File Not Found`.
    """
    cleaned = raw_path.strip().replace("\\", "/")
    p = Path(cleaned)
    if p.is_absolute():
        return p.as_posix()
    candidate = repo_root / cleaned
    if candidate.exists() or cleaned.startswith(".trellis/tasks/"):
        return candidate.resolve().as_posix()
    return cleaned


def _build_worker_context_manifest(
    repo_root: Path,
    task_rel: str,
    is_reviewer: bool,
) -> str:
    task_abs = (repo_root / task_rel).resolve()
    if not task_abs.is_dir():
        return ""

    jsonl_primary = "check.jsonl" if is_reviewer else "implement.jsonl"
    curated = _read_curated_jsonl(task_abs, jsonl_primary)
    if not curated and is_reviewer:
        curated = _read_curated_jsonl(task_abs, "implement.jsonl")

    artifacts = [
        (task_abs / name).as_posix()
        for name in ("prd.md", "design.md", "implement.md")
        if (task_abs / name).is_file()
    ]

    lines = [
        "",
        "---",
        INJECT_MARKER,
        f"[Trellis Auto-Injected Context | Active task: {task_rel}]",
        "1. Mandatory task artifacts (read BEFORE acting):",
    ]
    if artifacts:
        for art in artifacts:
            lines.append(f"   - {art}")
    else:
        lines.append(f"   - {task_abs.as_posix()}/ (inspect task directory)")

    lines.append("2. Mandatory project specs & research (read BEFORE acting):")
    if curated:
        for f_path, reason in curated:
            resolved_path = _to_manifest_path(repo_root, f_path)
            lines.append(f"   - {resolved_path} ({reason})")
    else:
        spec_dir_abs = (repo_root / ".trellis" / "spec").resolve().as_posix()
        lines.append(
            "   - No curated jsonl entries found; load the `trellis-before-dev` skill "
            f"or read `{spec_dir_abs}/` index files for every package/layer you touch."
        )

    if is_reviewer:
        lines.extend(
            [
                "3. Reviewer / Verifier guardrails:",
                "   - Check all diffs item-by-item against the specs & artifacts above.",
                "   - Run the project's test/lint/build quality gates to verify.",
                "   - Report issues to the owning worker by @name; do NOT edit files owned by active authors or `.trellis/**`.",
            ]
        )
    else:
        lines.extend(
            [
                "3. Worker / Author guardrails:",
                "   - Follow every spec & contract listed above; run local lint/test/build checks before reporting done.",
                "   - NEVER modify `.trellis/**` files (task state & specs belong exclusively to the main session).",
                "   - If running in `workspace:\"share\"`, commit cleanly on your worker branch only; never push to remote.",
            ]
        )

    return "\n".join(lines)


def handle_pre_tool_use(repo_root: Path, data: dict[str, Any]) -> dict[str, Any]:
    tool_call = data.get("toolCall")
    if not isinstance(tool_call, dict):
        return {"decision": "allow"}

    tool_name = _str(tool_call.get("name")).lower()
    args = tool_call.get("args")
    if not isinstance(args, dict):
        return {"decision": "allow"}

    _ensure_trellis_imports(repo_root)
    from common.active_task import resolve_active_task, resolve_context_key  # type: ignore[import-not-found]

    # Case A: Shell command calling task.py -> bridge session identity
    if "command" in tool_name:
        cmd = _str(args.get("CommandLine") or args.get("command"))
        mentions, has_create, subcommands = _extract_task_py_info(cmd)
        if mentions:
            ctx_key = resolve_context_key(data, platform="antigravity")
            if ctx_key:
                if subcommands:
                    _write_shell_ticket(repo_root, data, ctx_key, cmd, subcommands)
                if has_create and "TRELLIS_CONTEXT_ID" not in cmd:
                    if sys.platform.startswith("win"):
                        rewritten = f"$env:TRELLIS_CONTEXT_ID='{ctx_key}'; {cmd}"
                    else:
                        rewritten = f"TRELLIS_CONTEXT_ID={shlex.quote(ctx_key)} {cmd}"
                    key_name = "CommandLine" if "CommandLine" in args else "command"
                    return {"decision": "allow", "overwrite": {key_name: rewritten}}
        return {"decision": "allow"}

    # Case B: AGY Native Swarm `add` -> auto-inject specs & task artifacts into `brief`
    if tool_name == "swarm" or tool_name.endswith("_swarm"):
        action = _str(args.get("action")).lower()
        if action != "add":
            return {"decision": "allow"}

        _record_coordinator(repo_root, _str(data.get("conversationId")))
        worker_name = _str(args.get("name")).lower()
        if (
            worker_name in ("narrator", "monitor")
            or worker_name.startswith("narrator-")
            or worker_name.startswith("monitor-")
        ):
            return {"decision": "allow"}

        brief = _str(args.get("brief"))
        if not brief or INJECT_MARKER in brief:
            return {"decision": "allow"}

        active = resolve_active_task(
            repo_root,
            data,
            platform="antigravity",
            allow_single_session_fallback=True,
        )
        if not active.task_path or active.stale:
            return {"decision": "allow"}

        is_reviewer = any(kw in worker_name for kw in _REVIEWER_KEYWORDS)
        manifest = _build_worker_context_manifest(repo_root, active.task_path, is_reviewer)
        if not manifest:
            return {"decision": "allow"}

        return {"decision": "allow", "overwrite": {"brief": f"{brief}{manifest}"}}

    # Case C: AGY `invoke_subagent` -> auto-inject specs & task artifacts into `Prompt`
    if "subagent" in tool_name and "Prompt" in args:
        type_name = _str(args.get("TypeName")).lower()
        if type_name in ("browser", "image-generator"):
            return {"decision": "allow"}

        _record_coordinator(repo_root, _str(data.get("conversationId")))
        prompt = _str(args.get("Prompt"))
        if not prompt or INJECT_MARKER in prompt:
            return {"decision": "allow"}

        active = resolve_active_task(
            repo_root,
            data,
            platform="antigravity",
            allow_single_session_fallback=True,
        )
        if not active.task_path or active.stale:
            return {"decision": "allow"}

        role_label = f"{type_name} {_str(args.get('Role')).lower()}"
        is_reviewer = any(kw in role_label for kw in _REVIEWER_KEYWORDS)
        manifest = _build_worker_context_manifest(repo_root, active.task_path, is_reviewer)
        if not manifest:
            return {"decision": "allow"}

        return {"decision": "allow", "overwrite": {"Prompt": f"{prompt}{manifest}"}}

    return {"decision": "allow"}


# =============================================================================
# Entrypoint (Fail-Open Guaranteed)
# =============================================================================


def main() -> int:
    mode = sys.argv[1].strip().lower() if len(sys.argv) > 1 else ""
    fallback: dict[str, Any] = {"decision": "allow"} if mode == "pre-tool-use" else {}

    if os.environ.get("TRELLIS_HOOKS") == "0" or os.environ.get("TRELLIS_DISABLE_HOOKS") == "1":
        print(json.dumps(fallback))
        return 0

    try:
        data = _read_stdin_json()
        repo_root = _resolve_repo_root(data)
        if repo_root is None:
            print(json.dumps(fallback))
            return 0

        if mode == "session-start":
            out = handle_session_start(repo_root, data)
        elif mode == "pre-invocation":
            out = handle_pre_invocation(repo_root, data)
        elif mode == "pre-tool-use":
            out = handle_pre_tool_use(repo_root, data)
        else:
            out = fallback

        print(json.dumps(out, ensure_ascii=False))
        return 0
    except Exception:
        # Fail-open: never block the user's conversation or tool execution on unexpected errors.
        print(json.dumps(fallback))
        return 0


if __name__ == "__main__":
    sys.exit(main())
