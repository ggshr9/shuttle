package config

// ProxyProviderConfig defines a remote or local proxy provider source.
type ProxyProviderConfig struct {
	Name        string             `yaml:"name" json:"name"`
	URL         string             `yaml:"url" json:"url"`
	Path        string             `yaml:"path,omitempty" json:"path,omitempty"`
	Interval    string             `yaml:"interval,omitempty" json:"interval,omitempty"`
	Filter      string             `yaml:"filter,omitempty" json:"filter,omitempty"`
	HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty" json:"health_check,omitempty"`
}

// RuleProviderConfig defines a remote or local rule provider source.
type RuleProviderConfig struct {
	Name     string `yaml:"name" json:"name"`
	URL      string `yaml:"url" json:"url"`
	Path     string `yaml:"path,omitempty" json:"path,omitempty"`
	Behavior string `yaml:"behavior" json:"behavior"`
	Interval string `yaml:"interval,omitempty" json:"interval,omitempty"`
}

// HealthCheckConfig configures periodic health checking for proxy providers.
type HealthCheckConfig struct {
	URL         string `yaml:"url,omitempty" json:"url,omitempty"`
	Interval    string `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timeout     string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Tolerance   int    `yaml:"tolerance,omitempty" json:"tolerance,omitempty"`
	ToleranceMS int    `yaml:"tolerance_ms,omitempty" json:"tolerance_ms,omitempty"`
}
