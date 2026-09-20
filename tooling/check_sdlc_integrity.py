#!/usr/bin/env python3
"""SDLC Integrity Checker Wrapper.

Delegates to tooling.task_cli check command.
"""

import sys
from pathlib import Path

# 确保项目根目录在 sys.path 中
project_root = Path(__file__).resolve().parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

from tooling.task_cli import TaskManager


def main() -> int:
    manager = TaskManager()
    passed = manager.check()
    return 0 if passed else 1


if __name__ == "__main__":
    sys.exit(main())
