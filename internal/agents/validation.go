// Package agents defines Codex and Claude runners.
package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/nibzard/looper-go/internal/utils"
)

// ValidateSummary validates a summary against a JSON schema file.
// If schemaPath is empty, only minimal validation is performed.
func ValidateSummary(summary *Summary, schemaPath string) error {
	if summary == nil {
		return errors.New("summary is nil")
	}

	// Try schema validation if path is provided
	if schemaPath != "" {
		absPath, err := filepath.Abs(schemaPath)
		if err != nil {
			return fmt.Errorf("invalid schema path: %w", err)
		}

		if _, err := os.Stat(absPath); err == nil {
			// Schema file exists, validate against it
			if err := validateSummaryWithSchema(summary, absPath); err != nil {
				return err
			}
			return nil
		}
	}

	// Fallback to minimal validation
	return validateSummaryMinimal(summary)
}

// validateSummaryWithSchema validates a summary against the JSON schema.
func validateSummaryWithSchema(summary *Summary, schemaPath string) error {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat = true

	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}

	// Marshal summary to JSON for validation
	summaryData, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}

	var summaryObj interface{}
	if err := json.Unmarshal(summaryData, &summaryObj); err != nil {
		return fmt.Errorf("unmarshal summary: %w", err)
	}

	if err := schema.Validate(summaryObj); err != nil {
		return mapSchemaErrorToSummaryValidationError(err)
	}

	return nil
}

// mapSchemaErrorToSummaryValidationError converts jsonschema ValidationError to SummaryValidationError.
func mapSchemaErrorToSummaryValidationError(err error) error {
	if err == nil {
		return nil
	}

	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return &SummaryValidationError{Message: err.Error()}
	}

	// Find the first useful error
	var result error
	collectSchemaValidationErrors(ve, &result)
	if result != nil {
		return result
	}

	return &SummaryValidationError{Message: err.Error()}
}

// collectSchemaValidationErrors recursively collects validation errors.
func collectSchemaValidationErrors(err *jsonschema.ValidationError, result *error) {
	if err == nil {
		return
	}

	if len(err.Causes) == 0 {
		path := utils.JSONPointerToPath(err.InstanceLocation)
		*result = &SummaryValidationError{
			Path:    path,
			Message: err.Message,
		}
		return
	}

	for _, cause := range err.Causes {
		if *result == nil {
			collectSchemaValidationErrors(cause, result)
		}
	}
}

// validateSummaryMinimal performs minimal validation without JSON schema.
func validateSummaryMinimal(summary *Summary) error {
	var errs []string

	// Check task_id is present or null
	if summary.TaskID == "" {
		// task_id can be null (empty string is treated as null for Go)
		// This is allowed per the schema
	}

	// Check status is a valid enum value
	validStatuses := map[string]bool{
		"done":    true,
		"blocked": true,
		"skipped": true,
	}
	if summary.Status != "" && !validStatuses[summary.Status] {
		errs = append(errs, fmt.Sprintf("invalid status %q, must be one of: done, blocked, skipped", summary.Status))
	}

	// Check that at least one meaningful field is set
	if summary.TaskID == "" && summary.Status == "" && summary.Summary == "" &&
		len(summary.Files) == 0 && len(summary.Blockers) == 0 {
		return errors.New("summary is empty")
	}

	if len(errs) > 0 {
		return &SummaryValidationError{Message: strings.Join(errs, "; ")}
	}

	return nil
}
