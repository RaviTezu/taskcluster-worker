// Package extpoints provides methods that engine plugins can register their
// implements with as an import side-effect.
package extpoints

import (
	"github.com/Sirupsen/logrus"
	"github.com/taskcluster/taskcluster-worker/engines"
	"github.com/taskcluster/taskcluster-worker/runtime"
)

// EngineOptions is a wrapper for the set of options/arguments given to
// an EngineProvider when an Engine is created.
//
// We pass all options as a single argument, so that we can add additional
// properties without breaking source compatibility.
type EngineOptions struct {
	Environment *runtime.Environment
	Log         *logrus.Entry
	Config      interface{}
	// Note: This is passed by-value for efficiency (and to prohibit nil), if
	// adding any large fields please consider adding them as pointers.
	// Note: This is intended to be a simple argument wrapper, do not add methods
	// to this struct.
}

// EngineProvider is the interface engine implementors must implement and
// register with extpoints.Register(provider, "EngineName")
//
// This function must return an Engine implementation, generally this will only
// be called once in an application. But implementors should aim to write
// reentrant code.
//
// Any error here will be fatal and likely cause the worker to stop working.
// If an implementor can determine that the platform isn't supported at
// compile-time it is recommended to not register the implementation.
type EngineProvider interface {
	NewEngine(options EngineOptions) (engines.Engine, error)

	// ConfigSchema returns the CompositeSchema that represents the engine
	// configuration
	ConfigSchema() runtime.CompositeSchema
}

// EngineProviderBase is a base struct that provides empty implementations of
// some methods for EngineProvider
//
// Implementors of EngineProvider should embed this struct to ensure forward
// compatibility when we add new optional method to EngineProvider.
type EngineProviderBase struct{}

// ConfigSchema returns an empty composite schema.
func (EngineProviderBase) ConfigSchema() runtime.CompositeSchema {
	return runtime.NewEmptyCompositeSchema()
}
