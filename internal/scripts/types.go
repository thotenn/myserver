package scripts

import "time"

// ScriptConfig defines a script that can be executed from the dashboard.
type ScriptConfig struct {
	Name            string            `yaml:"name" json:"name"`
	Command         string            `yaml:"command" json:"command"`
	Description     string            `yaml:"description,omitempty" json:"description,omitempty"`
	Args            []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env             map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Timeout         int               `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	RequireConfirm  bool              `yaml:"requireConfirm,omitempty" json:"requireConfirm,omitempty"`
	Icon            string            `yaml:"icon,omitempty" json:"icon,omitempty"`
	AllowConcurrent bool              `yaml:"allowConcurrent,omitempty" json:"allowConcurrent,omitempty"`
	LogOutput       bool              `yaml:"logOutput,omitempty" json:"logOutput,omitempty"`

	// Resolved path after validation (not exposed in YAML/JSON)
	resolvedPath string
}

// Execution represents a single script execution.
type Execution struct {
	Name      string        `json:"name"`
	Status    string        `json:"status"` // "running", "success", "error", "timeout"
	Output    string        `json:"output"`
	ExitCode  int           `json:"exitCode"`
	StartedAt time.Time     `json:"startedAt"`
	Duration  time.Duration `json:"duration"`
	ClientIP  string        `json:"clientIP"`
}

// ScriptStatus is the public status of a script (for API responses).
type ScriptStatus struct {
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Icon           string `json:"icon,omitempty"`
	Running        bool   `json:"running"`
	RequireConfirm bool   `json:"requireConfirm,omitempty"`
	LastStatus     string `json:"lastStatus,omitempty"`
	LastExitCode   int    `json:"lastExitCode,omitempty"`
}
