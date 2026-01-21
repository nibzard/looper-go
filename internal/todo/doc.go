// Package todo parses, validates, and updates task files.
//
// The task file format (to-do.json) follows the schema defined in to-do.schema.json:
//
//	{
//	  "schema_version": 1,
//	  "source_files": ["README.md", "MIGRATION.md"],
//	  "tasks": [
//	    {
//	      "id": "T001",
//	      "title": "Task title",
//	      "priority": 1,
//	      "status": "todo",
//	      "details": "Optional details",
//	      "steps": ["step1", "step2"],
//	      "blockers": ["blocker reason"],
//	      "tags": ["tag1"],
//	      "files": ["file1.go"],
//	      "depends_on": ["T002"],
//	      "created_at": "2024-01-01T00:00:00Z",
//	      "updated_at": "2024-01-01T00:00:00Z"
//	    }
//	  ]
//	}
//
// # Validation
//
// The package supports two validation modes:
//
// 1. JSON Schema validation (when a schema file is provided):
//   - Full validation against JSON Schema draft-2020-12
//   - Supports: type checking, required fields, enum values, const, min/max, additionalProperties
//
// 2. Minimal fallback validation (when no schema is available):
//   - Basic structural checks (schema_version, source_files, tasks presence)
//   - Task field validation (id, title, priority range, status enum)
//   - No external dependencies required
//
// # Task Status Values
//
//   - "todo": Task is pending
//   - "doing": Task is currently being worked on
//   - "blocked": Task is blocked (see blockers field)
//   - "done": Task is complete
//
// # Priority Range
//
//   - 1: Highest priority
//   - 5: Lowest priority
//
// # File Format
//
// When writing task files, the package uses:
//   - 2-space indentation
//   - Trailing newline
//   - Stable key ordering (via JSON marshaling)
package todo
