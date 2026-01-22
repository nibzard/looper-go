#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TMP_DIR=$(mktemp -d)
PROJECT_DIR="$TMP_DIR/project"
STUB_BIN="$TMP_DIR/bin"
RUN_LOG="$TMP_DIR/run.log"

cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

mkdir -p "$PROJECT_DIR" "$STUB_BIN"

# Copy prompts for the Go binary
cp -r "$ROOT_DIR/prompts" "$PROJECT_DIR/prompts"

log_contains() {
    local pattern="$1"
    local file="$2"
    if command -v rg >/dev/null 2>&1; then
        rg -q "$pattern" "$file"
    else
        grep -q "$pattern" "$file"
    fi
}

cat > "$STUB_BIN/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

# Stub codex agent for smoke testing
# Handles: codex exec --json [--output-last-message PATH] [-]

last_message=""
json_output=0

# Skip "exec" command
if [ "${1:-}" = "exec" ]; then
    shift
fi

while [ "$#" -gt 0 ]; do
    case "$1" in
        --output-last-message)
            last_message="$2"
            shift 2
            ;;
        --json)
            json_output=1
            shift
            ;;
        -m|-c)
            shift 2  # Skip model and reasoning flags
            ;;
        -)
            shift
            break
            ;;
        *)
            shift
            ;;
    esac
done

# Consume stdin prompt
cat >/dev/null || true

# Create output in project directory (current dir when codex runs)
printf "smoke test readme\n" > README.md

summary='{"task_id":"T2","status":"done","summary":"smoke","files":["README.md"],"blockers":[]}'
if [ -n "$last_message" ]; then
    printf '%s\n' "$summary" > "$last_message"
fi

if [ "$json_output" -eq 1 ]; then
    printf '{"type":"assistant_message","message":{"content":[{"type":"text","text":"smoke"}]}}\n'
else
    printf '%s\n' "$summary"
fi
EOF

chmod +x "$STUB_BIN/codex"

cat > "$PROJECT_DIR/PROJECT.md" <<'EOF'
# Smoke Test Project

Goal: verify the looper can run one iteration and update to-do.json.
EOF

cat > "$PROJECT_DIR/to-do.json" <<'EOF'
{
  "schema_version": 1,
  "source_files": ["PROJECT.md"],
  "tasks": [
    {
      "id": "T10",
      "title": "Create hello.txt",
      "priority": 1,
      "status": "todo"
    },
    {
      "id": "T2",
      "title": "Add README.md",
      "priority": 1,
      "status": "todo"
    }
  ]
}
EOF

(
    cd "$PROJECT_DIR"
    PATH="$STUB_BIN:$PATH" \
        LOOPER_GIT_INIT=0 \
        LOOPER_MAX_ITERATIONS=1 \
        "$ROOT_DIR/bin/looper" run | tee "$RUN_LOG"
)

# Check for T2 in JSONL summary output
log_contains '"task_id":"T2"' "$RUN_LOG"
# Verify README.md was created by the stub agent
test -f "$PROJECT_DIR/README.md"
# Verify to-do.schema.json was created by bootstrap
test -f "$PROJECT_DIR/to-do.schema.json"
# Verify task T2 status is done
jq -e '.tasks[] | select(.id == "T2" and .status == "done")' "$PROJECT_DIR/to-do.json" >/dev/null

echo "Smoke test passed."
