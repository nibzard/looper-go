#!/usr/bin/env python3
"""
Claude stream-json output parser.

Extracts summary JSON from Claude's stream-json format.
The parser reads the raw output from stdin and extracts the summary.
"""

import json
import sys
import re
from typing import Optional, Dict, Any


def extract_json_block(text: str) -> Optional[str]:
    """Extract JSON object from text, handling markdown code blocks."""
    text = text.strip()

    # Try to unescape if the string contains escaped characters
    if "\\n" in text or "\\\"" in text:
        try:
            text = json.loads(text)
        except:
            pass

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


def parse_summary_from_line(line: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """Try to parse a summary from a single JSON line."""
    # Check if this looks like a summary by checking for known fields
    has_task_id = "task_id" in line
    has_status = "status" in line
    has_summary = "summary" in line
    has_files = "files" in line
    has_blockers = "blockers" in line

    if not (has_task_id or has_status or has_summary or has_files or has_blockers):
        return None

    # Normalize task_id (null -> empty string)
    if line.get("task_id") is None:
        line["task_id"] = ""

    # Check if summary has actual content
    if not line.get("task_id") and not line.get("status") and not line.get("summary"):
        if not line.get("files") and not line.get("blockers"):
            return None

    return line


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


def parse_claude_stream_json(output: str) -> Optional[Dict[str, Any]]:
    """Parse Claude's stream-json format and extract the summary."""
    last_assistant_content = ""
    last_message_content = ""

    for line in output.strip().split('\n'):
        if not line.strip():
            continue

        try:
            data = json.loads(line)
        except json.JSONDecodeError:
            continue

        # Check for assistant_message events with plain text content
        if data.get("type") == "assistant_message":
            content = data.get("content", "")
            if content:
                # Try to unescape if it's a JSON-encoded string
                try:
                    unescaped = json.loads(content)
                    content = unescaped
                except:
                    pass

                # Extract JSON from content
                json_str = extract_json_block(content)
                if json_str:
                    try:
                        obj = json.loads(json_str)
                        if "task_id" in obj and "status" in obj:
                            # This is a valid summary JSON
                            return obj
                    except:
                        pass

                # Accumulate non-JSON content
                if not last_assistant_content:
                    last_assistant_content = content

        # Look for full message events
        if "message" in data:
            text = extract_text_from_message(data)
            if text:
                last_message_content = text

        # Check for stream deltas
        if data.get("type") == "content_block_delta":
            delta = data.get("delta", {})
            if isinstance(delta, dict) and "text" in delta:
                last_message_content += delta["text"]

    # Prefer assistant_message content first
    if last_assistant_content:
        json_str = extract_json_block(last_assistant_content)
        if json_str:
            try:
                summary = json.loads(json_str)
                if parse_summary_from_line(summary):
                    return summary
            except:
                pass

    # Fall back to accumulated message content
    if last_message_content:
        json_str = extract_json_block(last_message_content)
        if json_str:
            try:
                summary = json.loads(json_str)
                if parse_summary_from_line(summary):
                    return summary
            except:
                pass

    return None


def main():
    # Read raw output from stdin
    output = sys.stdin.read()

    # Try to parse summary from Claude's stream-json format
    summary = parse_claude_stream_json(output)

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
