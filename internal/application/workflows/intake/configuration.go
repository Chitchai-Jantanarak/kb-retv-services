package intake

import "context"

// Configuration holds company-owned policy for mail assessment.
type Configuration struct {
	EvaluationDisabled bool
	RulesDisabled      bool
	AIEnabled          bool
	AutoCreateEnabled  bool
	Instructions       string
}

// ConfigurationSource resolves policy and shared tenant instructions.
type ConfigurationSource interface {
	ConfigurationFor(context.Context, int64) (Configuration, error)
}

// WithConfiguration connects mail policy to the extractor.
func WithConfiguration(source ConfigurationSource) Option {
	return func(e *Extractor) { e.configuration = source }
}
