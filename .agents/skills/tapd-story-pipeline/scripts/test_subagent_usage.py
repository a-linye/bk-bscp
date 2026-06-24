"""tests for subagent_usage.py — PostToolUse hook for tapd-story-pipeline cost tracking"""
import json
import os
import subprocess
import sys
import tempfile
import unittest


SCRIPT = os.path.join(os.path.dirname(__file__), "subagent_usage.py")


def run_hook(payload: dict, project_dir: str) -> dict:
    """以子进程方式调用 subagent_usage.py，stdin 喂入 payload JSON。

    钩子用 CLAUDE_PROJECT_DIR 解析 work_dir/iter_dir 的绝对路径，因此测试
    必须把它指向临时目录而非依赖 payload.cwd。
    """
    env = dict(os.environ, CLAUDE_PROJECT_DIR=project_dir)
    proc = subprocess.run(
        [sys.executable, SCRIPT],
        input=json.dumps(payload).encode("utf-8"),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=10,
        env=env,
    )
    assert proc.returncode == 0, f"hook exited {proc.returncode}: {proc.stderr.decode()}"
    return json.loads(proc.stdout.decode().strip())


class TestSpeckitExecutorAgent(unittest.TestCase):
    def test_executor_agent_marker_appends_cost_events(self):
        """回归：真实子代理名 speckit-executor-agent 必须命中分发并落盘。

        历史 bug：钩子曾误判 speckit-execution-agent，导致正常 specify/plan/
        implement 流程全部走 skip 分支，cost-events.jsonl 永不追加。
        """
        with tempfile.TemporaryDirectory() as project_dir:
            work_dir_rel = "specs/v0.9.x/1234567"
            work_dir_abs = os.path.join(project_dir, work_dir_rel)
            os.makedirs(work_dir_abs)

            prompt = (
                "你是隔离的 speckit 执行单元。\n"
                f"[spec-cost-marker] work_dir={work_dir_rel} "
                "stage=specify attempt=1 round=1 ts=2026-05-22T16:30:00+08:00\n"
            )
            payload = {
                "session_id": "abcdef123",
                "tool_name": "Task",
                "tool_input": {
                    "subagent_name": "speckit-executor-agent",
                    "prompt": prompt,
                },
                "tool_response": {
                    "type": "task_tool_result",
                    "toolCallBrief": "Execution Summary: 9 tool uses, cost: 33.95s",
                    "usage": {
                        "inputTokens": 68485,
                        "outputTokens": 3433,
                        "totalTokens": 71918,
                        "cacheTokens": 15360,
                        "cachedWriteTokens": 0,
                        "cachedMissTokens": 53125,
                        "credit": 1.2,
                    },
                },
            }

            result = run_hook(payload, project_dir)
            self.assertEqual(result, {"continue": True})

            jsonl_path = os.path.join(work_dir_abs, "cost-events.jsonl")
            self.assertTrue(os.path.isfile(jsonl_path), "cost-events.jsonl not created")

            with open(jsonl_path) as f:
                lines = f.readlines()
            self.assertEqual(len(lines), 1)
            record = json.loads(lines[0])
            self.assertEqual(record["work_dir"], work_dir_rel)
            self.assertEqual(record["stage"], "specify")
            self.assertEqual(record["attempt"], 1)
            self.assertEqual(record["round"], 1)
            self.assertEqual(record["ts_marker"], "2026-05-22T16:30:00+08:00")
            self.assertEqual(record["session_id"], "abcdef123")
            self.assertEqual(record["subagent_name"], "speckit-executor-agent")
            self.assertAlmostEqual(record["duration_sec"], 33.95, places=2)
            self.assertEqual(record["usage"]["input_tokens"], 68485)
            self.assertEqual(record["usage"]["credit"], 1.2)
            self.assertIn("ts_event", record)

    def test_code_reviewer_also_routes_to_speckit_handler(self):
        with tempfile.TemporaryDirectory() as project_dir:
            work_dir_rel = "wd"
            os.makedirs(os.path.join(project_dir, work_dir_rel))
            prompt = (
                f"[spec-cost-marker] work_dir={work_dir_rel} "
                "stage=validate attempt=2 round=3 ts=2026-05-22T16:40:00+08:00"
            )
            run_hook({
                "session_id": "s",
                "tool_name": "Task",
                "tool_input": {
                    "subagent_name": "code-reviewer",
                    "prompt": prompt,
                },
                "tool_response": {
                    "toolCallBrief": "Execution Summary: 2 tool uses",
                    "usage": {"inputTokens": 100, "outputTokens": 50, "credit": 0.1},
                },
            }, project_dir)
            jsonl = os.path.join(project_dir, work_dir_rel, "cost-events.jsonl")
            with open(jsonl) as f:
                record = json.loads(f.readline())
            self.assertEqual(record["duration_sec"], 0.0)
            self.assertEqual(record["stage"], "validate")
            self.assertEqual(record["subagent_name"], "code-reviewer")


class TestTapdPipelineAgent(unittest.TestCase):
    def test_pipeline_marker_appends_pipeline_cost_events(self):
        with tempfile.TemporaryDirectory() as project_dir:
            iter_dir_rel = "specs/v0.9.x"
            os.makedirs(os.path.join(project_dir, iter_dir_rel))
            prompt = (
                f"[pipeline-cost-marker] iter_dir={iter_dir_rel} story_id=1234567 "
                "action=execute ts=2026-05-26T11:30:00+08:00"
            )
            run_hook({
                "session_id": "sess-1",
                "tool_name": "Agent",
                "tool_input": {
                    "subagent_name": "tapd-pipeline-agent",
                    "prompt": prompt,
                },
                "tool_response": {
                    "toolCallBrief": "Execution Summary: 16 tool uses, cost: 48.83s",
                    "usage": {"inputTokens": 103491, "outputTokens": 4238, "credit": 0},
                },
            }, project_dir)
            jsonl = os.path.join(project_dir, iter_dir_rel, "pipeline-cost-events.jsonl")
            with open(jsonl) as f:
                record = json.loads(f.readline())
            self.assertEqual(record["iter_dir"], iter_dir_rel)
            self.assertEqual(record["story_id"], "1234567")
            self.assertEqual(record["action"], "execute")
            self.assertEqual(record["subagent_name"], "tapd-pipeline-agent")
            self.assertAlmostEqual(record["duration_sec"], 48.83, places=2)


class TestHookEdgeCases(unittest.TestCase):
    def test_non_task_event_silently_skipped(self):
        with tempfile.TemporaryDirectory() as project_dir:
            result = run_hook({
                "tool_name": "Read",
                "tool_input": {"prompt": "[spec-cost-marker] work_dir=x stage=y attempt=1 round=1 ts=z"},
            }, project_dir)
            self.assertEqual(result, {"continue": True})

    def test_unknown_subagent_silently_skipped(self):
        with tempfile.TemporaryDirectory() as project_dir:
            work_dir_rel = "wd"
            os.makedirs(os.path.join(project_dir, work_dir_rel))
            prompt = (
                f"[spec-cost-marker] work_dir={work_dir_rel} "
                "stage=specify attempt=1 round=1 ts=2026-05-22T16:30:00+08:00"
            )
            result = run_hook({
                "tool_name": "Task",
                "tool_input": {
                    "subagent_name": "some-other-agent",
                    "prompt": prompt,
                },
                "tool_response": {"usage": {"inputTokens": 1}},
            }, project_dir)
            self.assertEqual(result, {"continue": True})
            self.assertFalse(
                os.path.isfile(os.path.join(project_dir, work_dir_rel, "cost-events.jsonl")),
                "未知 subagent 不应落盘",
            )

    def test_no_marker_silently_skipped(self):
        with tempfile.TemporaryDirectory() as project_dir:
            result = run_hook({
                "tool_name": "Task",
                "tool_input": {
                    "subagent_name": "speckit-executor-agent",
                    "prompt": "随便一段没有 marker 的 prompt",
                },
                "tool_response": {"usage": {"inputTokens": 1}},
            }, project_dir)
            self.assertEqual(result, {"continue": True})

    def test_work_dir_not_exist_silently_skipped(self):
        with tempfile.TemporaryDirectory() as project_dir:
            prompt = (
                "[spec-cost-marker] work_dir=does/not/exist "
                "stage=specify attempt=1 round=1 ts=2026-05-22T00:00:00+08:00"
            )
            result = run_hook({
                "tool_name": "Task",
                "tool_input": {
                    "subagent_name": "speckit-executor-agent",
                    "prompt": prompt,
                },
                "tool_response": {"usage": {}},
            }, project_dir)
            self.assertEqual(result, {"continue": True})

    def test_malformed_payload_silently_continues(self):
        proc = subprocess.run(
            [sys.executable, SCRIPT],
            input=b"not a json",
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=10,
            env=dict(os.environ, CLAUDE_PROJECT_DIR=tempfile.gettempdir()),
        )
        self.assertEqual(proc.returncode, 0)
        self.assertEqual(json.loads(proc.stdout.decode().strip()), {"continue": True})


if __name__ == "__main__":
    unittest.main()
