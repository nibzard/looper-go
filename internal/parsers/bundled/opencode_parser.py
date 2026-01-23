#!/usr/bin/env python3
"""
OpenCode output parser.

This is an example parser for the OpenCode agent.
Since OpenCode's output format may vary, this parser provides a flexible approach.

If OpenCode has a specific output format, modify this parser accordingly.
"""

import json
import sys
from typing import Optional, Dict, Any


def extract_json_block(text: str) -> Optional[str]:
    """Extract JSON object from text, handling markdown code blocks."""
    text = text.strip()

    # Check for markdown code block
    if "```json" in text:
        start = text.find("{")
        end = text.rfind("}")
        if start >= 0 and end > start:
            return text[start:end+1]

    # Check for code block without language tag
    if "```" in text:
        start = text.find("{")
        end = text.rfind("}")
        if start >= 0 and end > start:
            return text[start:end+1]

    # Check if the whole string is JSON
    if text.startswith("{") and text.endswith("}"):
        return text

    # Look for first JSON object in the string
    start = text.find("{")
    if start >= 0:
        # Find matching closing brace
        brace_count = 0
        for i in range(start, len(text)):
            if text[i] == '{':
                brace_count += 1
            elif text[i] == '}':
                brace_count -= 1
                if brace_count == 0:
                    return text[start:i+1]

    return ""


def parse_summary_from_raw(raw: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """Try to parse a summary from a raw JSON object."""
    # Check if this looks like a summary by checking for known fields
    has_task_id = "task_id" in raw
    has_status = "status" in raw
    has_summary_text = "summary" in raw
    has_files = "files" in raw
    has_blockers = "blockers" in raw

    if not (has_task_id or has_status or has_summary_text or has_files or has_blockers):
        return None

    # Normalize task_id (null -> empty string)
    if raw.get("task_id") is None:
        raw["task_id"] = ""

    # Check if summary has actual content
    if not raw.get("task_id") and not raw.get("status") and not raw.get("summary"):
        if not raw.get("files") and not raw.get("blockers"):
            return None

    return raw


def parse_opencode_output(output: str) -> Optional[Dict[str, Any]]:
    """Parse OpenCode output and extract the summary."""
    # Try to find a summary JSON in the output
    # This approach works for many agent output formats

    # First, try to parse the entire output as JSON
    try:
        data = json.loads(output.strip())
        summary = parse_summary_from_raw(data)
        if summary:
            return summary
    except:
        pass

    # Try to extract JSON from the output
    json_str = extract_json_block(output)
    if json_str:
        try:
            obj = json.loads(json_str)
            summary = parse_summary_from_raw(obj)
            if summary:
                return summary
        except:
            pass

    # Try line-by-line parsing for NDJSON
    for line in output.strip().split('\n'):
        if not line.strip():
            continue
        try:
            data = json.loads(line)
            summary = parse_summary_from_raw(data)
            if summary:
                return summary
        except:
            continue

    return None


def main():
    # Read raw output from stdin
    output = sys.stdin.read()

    # Try to parse summary from OpenCode output
    summary = parse_opencode_output(output)

    if summary:
        # Output the summary as JSON
        json.dump(summary, sys.stdout)
        sys.exit(0)
    else:
        # No summary found - this is not an error, just indicates no summary
        json.dump({"error": "no summary found"}, sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
