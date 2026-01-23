# Looper-Go Architecture

**Looper-Go** is a deterministic, autonomous loop runner for AI agents (Codex, Claude). It processes exactly one task per iteration from a JSON backlog, with fresh context each run, and maintains a JSONL audit log for traceability.

## Table of Contents

1. [High-Level Overview](#high-level-overview)
2. [Core Workflow](#core-workflow)
3. [Project Structure](#project-structure)
4. [CLI Architecture](#cli-architecture)
5. [Configuration System](#configuration-system)
6. [Task Management](#task-management)
7. [Agent System](#agent-system)
8. [Loop Orchestration](#loop-orchestration)
9. [Logging System](#logging-system)
10. [Data Flows](#data-flows)

---

## High-Level Overview

```mermaid
graph TB
    subgraph "User Layer"
        A[CLI Command] --> B[Config]
        A --> C[Task File]
    end

    subgraph "Core Engine"
        D[Loop Orchestrator] --> E[Agent Runner]
        D --> F[Task Manager]
        D --> G[Logger]
    end

    subgraph "External Tools"
        H[Codex Binary]
        I[Claude Binary]
        J[Custom Agents]
    end

    B --> D
    C --> F
    E --> H
    E --> I
    E --> J

    D --> K[JSONL Logs]
    D --> L[Updated Task File]
    D --> M[Hooks]
```

**Core Principles:**
- **Deterministic**: One task per iteration, no ambiguity
- **Autonomous**: Runs until completion or max iterations
- **Traceable**: Full JSONL audit log of all operations
- **Fresh Context**: Each agent run starts with current project state
- **Repairable**: Can recover from invalid states

---

## Core Workflow

```mermaid
stateDiagram-v2
    [*] --> Initialize: Start
    Initialize --> LoadConfig: Parse CLI args
    LoadConfig --> LoadTodo: Open task file

    LoadTodo --> Bootstrap: File missing/empty
    LoadTodo --> Validate: File exists

    Bootstrap --> Validate: Agent creates tasks

    Validate --> RunLoop: Valid
    Validate --> Repair: Invalid

    Repair --> Validate: Agent repairs file

    state RunLoop {
        [*] --> SelectTask
        SelectTask --> RunAgent: Task found
        SelectTask --> Review: No tasks

        RunAgent --> ApplyResults
        ApplyResults --> RunHooks
        RunHooks --> SelectTask
    }

    RunLoop --> [*]: Max iterations / Complete
    Review --> [*]: Review pass done
```

**Workflow Steps:**

1. **Initialize** - Load configuration from hierarchy (defaults → user → project → env → CLI)
2. **Load Todo** - Read and parse the task file (to-do.json)
3. **Bootstrap** - If missing, use agent to create initial task list from project docs
4. **Validate** - Check schema, dependencies, and data integrity
5. **Repair** - If invalid, use agent to fix the task file
6. **Run Loop** - Main iteration cycle:
   - Select next task (doing → todo → blocked priority)
   - Mark as "doing"
   - Run agent with current context
   - Apply results (update status, add new tasks)
   - Execute post-iteration hooks
7. **Review** - When tasks exhausted, run review pass
8. **Complete** - Add completion marker and exit

---

## Project Structure

```
looper-go/
├── cmd/
│   ├── looper/
│   │   └── main.go          # Entry point, signal handling
│   └── root.go              # Command router, flag parsing
│
├── internal/
│   ├── config/              # Configuration loading
│   │   ├── config.go        # Main config structs
│   │   └── loader.go        # Hierarchical loading logic
│   │
│   ├── agents/              # Agent system
│   │   ├── agents.go        # Registry, interfaces, runners
│   │   └── agents_test.go
│   │
│   ├── loop/                # Core orchestration
│   │   ├── loop.go          # Main loop state machine
│   │   ├── select.go        # Task selection algorithm
│   │   └── loop_test.go
│   │
│   ├── todo/                # Task file handling
│   │   ├── todo.go          # Parsing, validation, updates
│   │   ├── schema.go        # JSON schema handling
│   │   └── todo_test.go
│   │
│   ├── prompts/             # Prompt templates
│   │   ├── prompts.go       # Template rendering
│   │   └── schema.go        # Prompt schema validation
│   │
│   ├── logging/             # JSONL logging
│   │   ├── logging.go       # Logger, log writers
│   │   └── tail.go          # Log tailing utility
│   │
│   ├── hooks/               # Post-iteration hooks
│   │   └── hooks.go         # Hook execution
│   │
│   ├── utils/               # Shared utilities
│   │   ├── doc.go
│   │   └── platform.go      # Platform detection
│   │
│   └── ui/                  # Terminal interfaces (optional)
│
├── prompts/                 # Bundled prompt templates
│   ├── bootstrap.md
│   ├── repair.md
│   ├── iteration.md
│   ├── review.md
│   └── push.md
│
├── bin/
│   └── looper.sh            # Legacy shell script (for reference)
│
├── to-do.json               # Task file (runtime)
├── run.schema.json          # Task file schema
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## CLI Architecture

```mermaid
graph TD
    A[looper] --> B{Subcommand}

    B --> C[run]
    B --> D[ls]
    B --> E[tail]
    B --> F[validate]
    B --> G[fmt]
    B --> H[config]
    B --> I[init]
    B --> J[push]
    B --> K[clean]
    B --> L[doctor]
    B --> M[completion]

    C --> C1[Main loop execution<br/>Default if no subcommand]
    D --> D1[List tasks by status]
    E --> E1[View JSONL logs]
    F --> F1[Validate task file]
    G --> G1[Format task file]
    H --> H1[Show effective config]
    I --> I1[Initialize new project]
    J --> J1[Release workflow]
    K --> K1[Clean old logs]
    L --> L1[Check dependencies]
    M --> M1[Shell completion setup]
```

### Command Hierarchy

| Command | Purpose | Key Functions |
|---------|---------|---------------|
| `run` | Execute main loop | `cmd.Run` |
| `ls` | List tasks | `cmd.ListTasks` |
| `tail` | View logs | `cmd.TailLogs` |
| `validate` | Validate task file | `cmd.ValidateFile` |
| `fmt` | Format task file | `cmd.FormatFile` |
| `config` | Show config | `cmd.ShowConfig` |
| `init` | Initialize project | `cmd.InitProject` |
| `push` | Release workflow | `cmd.PushRelease` |
| `clean` | Clean logs | `cmd.CleanLogs` |
| `doctor` | Check dependencies | `cmd.RunDoctor` |

---

## Configuration System

Configuration is loaded hierarchically, with each layer overriding the previous:

```mermaid
graph TD
    A[1. Defaults] --> B[2. User Config File]
    B --> C[3. Project Config File]
    C --> D[4. Environment Variables]
    D --> E[5. CLI Arguments]

    E --> F[Final Configuration]

    F --> G[Agent Settings]
    F --> H[Loop Settings]
    F --> I[Path Settings]
    F --> J[Hook Settings]

    G --> K[Codex Config]
    G --> L[Claude Config]
    G --> M[Custom Agents]
```

### Configuration Sources

| Priority | Source | Location | Format |
|----------|--------|----------|--------|
| 1 (lowest) | Defaults | Compiled-in | Go structs |
| 2 | User config | `~/.config/looper/config.json` | JSON |
| 3 | Project config | `.looper/config.json` | JSON |
| 4 | Environment | `LOOPER_*` variables | Env vars |
| 5 (highest) | CLI args | Command line | Flags |

### Agent Configuration Structure

```go
type AgentConfig struct {
    Binary    string       // Path to agent binary
    Model     string       // Model name/ID
    Args      []string     // Additional arguments
    Timeout   time.Duration // Execution timeout
    WorkDir   string       // Working directory
    LastMsg   string       // Path to last message file
}
```

### Scheduling Strategies

```mermaid
graph LR
    A[Scheduler] --> B{Strategy}
    B --> C[Single]
    B --> D[Odd-Even]
    B --> E[Round-Robin]

    C --> C1[Always use Agent X]
    D --> D1[Odd: Agent A<br/>Even: Agent B]
    E --> E1[A → B → C → A ...]
```

---

## Task Management

### Task File Structure

```json
{
  "schema_version": 1,
  "project": {
    "name": "Project Name",
    "root": "."
  },
  "source_files": ["README.md"],
  "tasks": [
    {
      "id": "T001",
      "title": "Task title",
      "description": "Detailed description",
      "reference": "path/to/relevant/files",
      "priority": 1,
      "status": "todo",
      "details": "Implementation notes",
      "steps": ["Step 1", "Step 2"],
      "blockers": ["Blocking issue description"],
      "tags": ["category", "feature"],
      "files": ["file1.go", "file2.go"],
      "depends_on": ["T002"],
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Task Status Flow

```mermaid
stateDiagram-v2
    [*] --> todo: Created
    todo --> doing: Selected for execution
    doing --> done: Completed successfully
    doing --> blocked: Encountered blocker
    blocked --> doing: Blocker resolved
    blocked --> todo: Re-queued
    done --> [*]
```

### Task Selection Algorithm

```mermaid
graph TD
    A[SelectTask] --> B{Has doing tasks?}
    B -->|Yes| C[Return lowest ID doing task]
    B -->|No| D{Has todo tasks?}

    D -->|Yes| E[Find highest priority<br/>Then lowest ID]
    D -->|No| F{Has blocked tasks?}

    F -->|Yes| G[Find highest priority<br/>Then lowest ID]
    F -->|No| H[Return nil - no tasks]

    E --> I{Dependencies<br/>satisfied?}
    G --> I

    I -->|Yes| J[Return task]
    I -->|No| K[Skip, check next]
```

**Priority:**
1. `doing` tasks (lowest ID first)
2. `todo` tasks (highest priority = lowest number, then lowest ID)
3. `blocked` tasks (highest priority = lowest number, then lowest ID)

### Dependency Validation

```mermaid
graph TD
    A[Validate Dependencies] --> B{Cycles detected?}
    B -->|Yes| C[Error: Circular dependency]
    B -->|No| D{Missing deps?}

    D -->|Yes| E[Error: Missing dependency]
    D -->|No| F[Valid]

    C --> G[Must fix manually or repair]
    E --> G
```

---

## Agent System

### Agent Architecture

```mermaid
graph TD
    A[Agent Interface] --> B[Agent Registry]

    B --> C[Codex Agent]
    B --> D[Claude Agent]
    B --> E[Custom Agents]

    A --> F[Run Method]
    A --> G[StreamLogs]

    F --> H[Build Command]
    F --> I[Execute Process]
    F --> J[Stream Output]
    F --> K[Extract Summary]
    F --> L[Validate Summary]

    H --> M[Binary + Args + Model]
    I --> N[Process Execution]
    J --> O[JSON Event Stream]
    K --> P[Parse Output]
    L --> Q[Schema Validation]
```

### Agent Registry

The agent system uses a registry pattern:

```go
type Agent interface {
    Run(ctx context.Context, prompt string) (*Summary, error)
}

var registry = map[string]Agent{
    "codex":  &CodexAgent{},
    "claude": &ClaudeAgent{},
}

func RegisterAgent(name string, agent Agent) {
    registry[name] = agent
}
```

### Agent Execution Flow

```mermaid
sequenceDiagram
    participant L as Loop
    participant A as Agent Runner
    participant B as Agent Binary
    participant W as Log Writer

    L->>A: Run agent with prompt
    A->>B: Execute process
    B->>A: Stream JSON events

    loop For each event
        A->>W: Write to JSONL log
        A->>L: Emit log event
    end

    B->>A: Exit with summary
    A->>A: Validate summary schema
    A->>L: Return validated summary
```

### Log Event Types

```mermaid
graph TD
    A[Agent Output] --> B{Event Type}

    B --> C[message]
    B --> D[tool_use]
    B --> E[command]
    B --> F[error]
    B --> G[summary]

    C --> H[Agent message content]
    D --> I[Tool invocation]
    E --> J[Command execution]
    F --> K[Error details]
    G --> L[Final summary]
```

---

## Loop Orchestration

### Loop State Machine

```mermaid
stateDiagram-v2
    [*] --> Initialized: New Loop()

    Initialized --> Bootstrap: Todo file missing
    Initialized --> Validated: Todo file exists

    Bootstrap --> Validated: Tasks created

    Validated --> Running: Start iteration

    state Running {
        [*] --> Selecting
        Selecting --> Executing: Task selected
        Selecting --> Reviewing: No tasks

        Executing --> Updating: Agent done
        Updating --> Hooking: Results applied
        Hooking --> Selecting: Continue
    }

    Running --> Completed: All tasks done
    Reviewing --> Completed: Review complete

    Completed --> [*]
```

### Iteration Cycle

```mermaid
graph TD
    A[Start Iteration] --> B[Select Task]
    B --> C{Task found?}

    C -->|No| D[Run Review Pass]
    C -->|Yes| E[Mark as doing]

    E --> F[Select Agent]
    F --> G[Build Prompt]
    G --> H[Run Agent]

    H --> I{Agent result}
    I -->|Success| J[Apply Summary]
    I -->|Error| K[Log error, continue]

    J --> L{New tasks?}
    L -->|Yes| M[Add to todo list]
    L -->|No| N[Update task status]

    N --> O[Execute Hooks]
    O --> P{More iterations?}

    P -->|Yes| B
    P -->|No| D

    D --> Q{Changes?}
    Q -->|Yes| B
    Q -->|No| R[Exit]
```

### Prompt Building

```mermaid
graph TD
    A[Build Prompt] --> B{Prompt Type}

    B --> C[Bootstrap]
    B --> D[Repair]
    B --> E[Iteration]
    B --> F[Review]
    B --> G[Push]

    C --> H[Project context]
    C --> I[Source files]

    D --> J[Invalid file]
    D --> K[Error details]

    E --> L[Current task]
    E --> M[Recent logs]
    E --> N[Task history]

    F --> O[All completed tasks]
    F --> P[Recent changes]

    G --> Q[Release notes]
    G --> R[Changelog]
```

---

## Logging System

### Log Architecture

```mermaid
graph TD
    A[RunLogger] --> B[Log Directory]
    A --> C[Run ID]

    B --> D[logs/]
    D --> E[runs/]
    E --> F[<run-id>/]
    F --> G[events.jsonl]

    J[Agent Events] --> K[JSON Lines]
    K --> G

    A --> L[Log Methods]
    L --> M[LogMessage]
    L --> N[LogTool]
    L --> O[LogCommand]
    L --> P[LogError]
    L --> Q[LogSummary]
```

### JSONL Log Format

Each line is a JSON object:

```json
{"type": "message", "timestamp": "2024-01-01T00:00:00Z", "role": "assistant", "content": "..."}
{"type": "tool_use", "timestamp": "2024-01-01T00:00:00Z", "name": "read_file", "input": {"file_path": "..."}}
{"type": "command", "timestamp": "2024-01-01T00:00:00Z", "command": "go build ./cmd/looper"}
{"type": "error", "timestamp": "2024-01-01T00:00:00Z", "error": "Build failed"}
{"type": "summary", "timestamp": "2024-01-01T00:00:00Z", "status": "done", "new_tasks": [...]}
```

### Log Organization

```
looper-logs/
├── runs/
│   ├── 2024-01-01_12-00-00/
│   │   └── events.jsonl
│   ├── 2024-01-01_13-30-45/
│   │   └── events.jsonl
│   └── ...
└── last
    └── -> symlink to latest run
```

---

## Data Flows

### Complete Run Flow

```mermaid
sequenceDiagram
    participant U as User
    participant C as CLI
    participant F as Config
    participant T as Todo
    participant L as Loop
    participant A as Agent
    participant B as Binary
    participant H as Hooks
    participant G as Git

    U->>C: looper run
    C->>F: Load config
    F-->>C: Config

    C->>T: Load to-do.json
    T-->>C: Tasks

    C->>L: New Loop(config, tasks)
    L->>L: Initialize logger

    loop Until done
        L->>T: Select next task
        T-->>L: Task

        L->>T: Mark as doing
        L->>A: Run agent
        A->>B: Execute with prompt
        B-->>A: Stream events
        A-->>L: Summary

        L->>T: Apply results
        T-->>L: Updated tasks
        L->>H: Execute hooks
    end

    L->>T: Mark review done
    T-->>G: Commit changes
```

### Configuration Loading Flow

```mermaid
sequenceDiagram
    participant C as CLI
    participant L as Loader
    participant D as Defaults
    participant U as User Config
    participant P as Project Config
    participant E as Environment
    participant A as Args

    C->>L: LoadConfig()
    L->>D: Get defaults
    D-->>L: Base config

    alt User config exists
        L->>U: Load ~/.config/looper/config.json
        U-->>L: Merge
    end

    alt Project config exists
        L->>P: Load .looper/config.json
        P-->>L: Merge
    end

    L->>E: Read LOOPER_* vars
    E-->>L: Merge

    L->>A: Parse CLI flags
    A-->>L: Merge

    L-->>C: Final config
```

### Task Update Flow

```mermaid
sequenceDiagram
    participant L as Loop
    participant S as Agent Summary
    participant T as Todo File
    participant V as Validator

    L->>S: ApplyTo(task, file)
    S->>S: Parse status changes
    S->>S: Extract new tasks

    alt New tasks
        S->>V: Validate new tasks
        V->>V: Check schema
        V->>V: Check dependencies
        V-->>S: Valid
        S->>T: Append tasks
    end

    S->>T: Update task status
    S->>T: Update timestamps
    T->>T: Write to disk

    S-->>L: Result
```

---

## Key Design Decisions

1. **JSONL for Logging**: Line-delimited JSON for easy streaming and parsing
2. **Fresh Context**: Each agent run sees current state, no long-running state
3. **Deterministic Selection**: Clear task ordering, no ambiguity
4. **Repairable**: Can recover from invalid states via agent repair
5. **Auditable**: Full log of all operations for debugging
6. **Extensible**: Agent registry allows adding new agent types
7. **Hierarchical Config**: Flexible configuration from multiple sources

---

## Extension Points

| Component | How to Extend |
|-----------|---------------|
| Agents | Implement `Agent` interface and register |
| Prompts | Add custom templates or use `--prompt-file` |
| Hooks | Add commands to `hooks.post_iteration` array |
| Schedulers | Add new strategy to config loader |
| Log Parsers | Process JSONL files with custom tools |

---

## Performance Considerations

- **Log Size**: JSONL logs grow with each run; use `looper clean` periodically
- **Context Window**: Large todo files may hit agent limits; consider splitting
- **Agent Latency**: Each iteration waits for agent completion; parallel agents not supported
- **File I/O**: Task file written after each iteration; SSD recommended

---

## Security Considerations

- **Command Execution**: Agents can run arbitrary commands; review hooks
- **API Keys**: Stored in config files; ensure proper permissions
- **Working Directory**: Agents run within project root; sandboxed by directory
- **Log Sensitivity**: Logs may contain sensitive data; clean before sharing

---

## Future Enhancements

Potential areas for expansion:

- Parallel agent execution for independent tasks
- Task prioritization heuristics
- Distributed task execution
- Web dashboard for monitoring
- Custom log analyzers
- Task templates and snippets
- Integration with project management tools
