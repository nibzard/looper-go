#!/usr/bin/env python3
"""
Codex NDJSON output parser.

Extracts summary JSON from Codex's NDJSON (newline-delimited JSON) format.
The parser reads the raw output from stdin and extracts the summary.
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

    return None


def extract_text_from_message(msg: Dict[str, Any]) -> str:
    """Extract text content from a message object."""
    if "message" in msg:
        content = msg["message"].get("content", "")
    else:
        content = msg.get("content", "")

    # Handle different content formats
    if isinstance(content, str):
        return content
    elif isinstance(content, dict):
        if "text" in content:
            return content["text"]
    elif isinstance(content, list):
        parts = []
        for item in content:
            if isinstance(item, str) and item:
                parts.append(item)
            elif isinstance(item, dict):
                if "text" in item:
                    text = item["text"]
                    if text:
                        parts.append(text)
        return "\n".join(parts)

    return ""


def parse_summary_from_raw(raw: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """Try to parse a summary from a raw JSON object."""
    # Check if this looks like a summary by checking for known fields
    has_task_id = "task_id" in raw
    has_status = "status" in raw
    has_summary = "summary" in raw
    has_files = "files" in raw
    has_blockers = "blockers" in raw

    if not (has_task_id or has_status or has_summary or has_files or has_blockers):
        return None

    # Normalize task_id (null -> empty string)
    if raw.get("task_id") is None:
        raw["task_id"] = ""

    # Check if summary has actual content
    if not raw.get("task_id") and not raw.get("status") and not raw.get("summary"):
        if not raw.get("files") and not raw.get("blockers"):
            return None

    return raw


def parse_codex_ndjson(output: str) -> Optional[Dict[str, Any]]:
    """Parse Codex's NDJSON format and extract the summary."""
    last_summary = None

    for line in output.strip().split('\n'):
        if not line.strip():
            continue

        try:
            data = json.loads(line)
        except json.JSONDecodeError:
            # Not JSON, skip
            continue

        # Try to parse summary directly from the JSON object
        summary = parse_summary_from_raw(data)
        if summary:
            last_summary = summary
            continue

        # Try to extract summary from text content
        text = extract_text_from_message(data)
        if text:
            json_str = extract_json_block(text)
            if json_str:
                try:
                    obj = json.loads(json_str)
                    summary = parse_summary_from_raw(obj)
                    if summary:
                        last_summary = summary
                except:
                    pass

    return last_summary


def main():
    # Read raw output from stdin
    output = sys.stdin.read()

    # Try to parse summary from Codex's NDJSON format
    summary = parse_codex_ndjson(output)

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
