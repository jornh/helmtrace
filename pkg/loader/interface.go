package loader

import "helmtrace/pkg/analyzer"

// Loader loads a set of ordered layers from some source.
// Implementations must return layers in precedence order: index 0 is lowest.
type Loader interface {
	Load() ([]analyzer.Layer, error)
}
