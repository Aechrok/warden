package domain

// ParamDefinition describes a single input parameter accepted by an action.
// Used by plugins to declare their action schema and by the UI to render forms.
type ParamDefinition struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Description string   `json:"description,omitempty"`
	Default     any      `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// ActionResult is the outcome of executing a plugin action against a single
// instance. Data carries provider-specific structured detail.
type ActionResult struct {
	Success bool           `json:"success"`
	Message string         `json:"message,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}
