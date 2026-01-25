package workflows

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/nibzard/looper-go/internal/plugin"
)

// Registry holds registered workflow types and their factories.
var Registry = struct {
	sync.RWMutex
	types map[WorkflowType]WorkflowFactory
}{
	types: make(map[WorkflowType]WorkflowFactory),
}

// Register registers a workflow type with its factory.
// Called in init() functions of workflow packages.
func Register(workflowType WorkflowType, factory WorkflowFactory) {
	Registry.Lock()
	defer Registry.Unlock()
	Registry.types[workflowType] = factory
}

// Unregister removes a workflow type from the registry.
// Primarily useful for testing.
func Unregister(workflowType WorkflowType) {
	Registry.Lock()
	defer Registry.Unlock()
	delete(Registry.types, workflowType)
}

// New creates a workflow of the specified type.
// It first checks the plugin registry, then falls back to the built-in registry.
// Returns WorkflowNotFoundError if the type is not registered.
func New(workflowType WorkflowType, cfg interface{}, workDir string, todoFile interface{}) (Workflow, error) {
	// First, check if there's a plugin for this workflow type
	pluginRegistry := plugin.GetRegistry()
	if pluginRegistry.IsInitialized() {
		if p := pluginRegistry.GetByWorkflowType(string(workflowType)); p != nil {
			// Use the plugin
			return NewPluginWorkflow(p, cfg, workDir, todoFile)
		}
	}

	// Fall back to built-in registry
	Registry.RLock()
	factory, ok := Registry.types[workflowType]
	Registry.RUnlock()

	if !ok {
		return nil, &WorkflowNotFoundError{WorkflowType: workflowType}
	}

	return factory(cfg, workDir, todoFile)
}

// List returns all registered workflow types in alphabetical order.
func List() []WorkflowType {
	Registry.RLock()
	defer Registry.RUnlock()

	types := make([]WorkflowType, 0, len(Registry.types))
	for t := range Registry.types {
		types = append(types, t)
	}

	sort.Slice(types, func(i, j int) bool {
		return strings.ToLower(string(types[i])) < strings.ToLower(string(types[j]))
	})

	return types
}

// IsRegistered returns true if the workflow type is registered.
func IsRegistered(workflowType WorkflowType) bool {
	Registry.RLock()
	defer Registry.RUnlock()
	_, ok := Registry.types[workflowType]
	return ok
}

// Describe returns a description map of all registered workflows.
func Describe() map[WorkflowType]string {
	Registry.RLock()
	defer Registry.RUnlock()

	result := make(map[WorkflowType]string, len(Registry.types))
	for t, factory := range Registry.types {
		// Create a dummy workflow to get its description
		// We pass nil since we just need the description
		// Workflow implementations should handle nil gracefully for description purposes
		if w, err := factory(nil, "", nil); err == nil && w != nil {
			result[t] = w.Description()
		} else {
			result[t] = fmt.Sprintf("Workflow %s", t)
		}
	}
	return result
}
