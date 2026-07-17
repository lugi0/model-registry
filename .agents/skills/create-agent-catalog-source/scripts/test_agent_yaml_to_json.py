"""Tests for agent_yaml_to_json.py — invoked as a CLI tool via subprocess."""

import json
import subprocess
import sys
from pathlib import Path

import pytest

SCRIPT = str(Path(__file__).parent / "agent_yaml_to_json.py")


def run_script(*args: str) -> subprocess.CompletedProcess:
    return subprocess.run(
        [sys.executable, SCRIPT, *args],
        capture_output=True,
        text=True,
    )


class TestValidConversion:
    def test_simple_agent_yaml(self, tmp_path):
        agent_yaml = tmp_path / "agent.yaml"
        agent_yaml.write_text(
            "name: my-agent\n"
            "framework: langgraph\n"
            'description: "A test agent"\n'
        )
        result = run_script(str(agent_yaml))
        assert result.returncode == 0
        parsed = json.loads(result.stdout.strip())
        assert parsed["name"] == "my-agent"
        assert parsed["framework"] == "langgraph"
        assert parsed["description"] == "A test agent"

    def test_keys_are_sorted(self, tmp_path):
        agent_yaml = tmp_path / "agent.yaml"
        agent_yaml.write_text("zebra: z\nalpha: a\nmiddle: m\n")
        result = run_script(str(agent_yaml))
        assert result.returncode == 0
        keys = list(json.loads(result.stdout.strip()).keys())
        assert keys == sorted(keys)

    def test_compact_separators(self, tmp_path):
        agent_yaml = tmp_path / "agent.yaml"
        agent_yaml.write_text("name: test\nvalue: 1\n")
        result = run_script(str(agent_yaml))
        assert result.returncode == 0
        output = result.stdout.strip()
        assert " " not in output

    def test_nested_structure(self, tmp_path):
        agent_yaml = tmp_path / "agent.yaml"
        agent_yaml.write_text(
            "name: nested-agent\n"
            "env:\n"
            "  required:\n"
            "    - API_KEY\n"
            "  optional:\n"
            "    - DEBUG\n"
        )
        result = run_script(str(agent_yaml))
        assert result.returncode == 0
        parsed = json.loads(result.stdout.strip())
        assert parsed["env"]["required"] == ["API_KEY"]
        assert parsed["env"]["optional"] == ["DEBUG"]


class TestErrorCases:
    def test_no_arguments(self):
        result = run_script()
        assert result.returncode != 0
        assert "Usage" in result.stderr

    def test_missing_file(self):
        result = run_script("/nonexistent/path/agent.yaml")
        assert result.returncode != 0
        assert "not found" in result.stderr

    def test_non_dict_yaml(self, tmp_path):
        agent_yaml = tmp_path / "agent.yaml"
        agent_yaml.write_text("- item1\n- item2\n")
        result = run_script(str(agent_yaml))
        assert result.returncode != 0
        assert "mapping" in result.stderr

    def test_invalid_yaml(self, tmp_path):
        agent_yaml = tmp_path / "agent.yaml"
        agent_yaml.write_text(":\n  bad: {\n  yaml: [unterminated\n")
        result = run_script(str(agent_yaml))
        assert result.returncode != 0
