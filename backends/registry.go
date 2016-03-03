package backends

import (
	"github.com/Graylog2/sidecar/api/graylog"
	"github.com/Graylog2/sidecar/util"
	"github.com/Graylog2/sidecar/context"
)

var log = util.Log()

type Backend interface {
	Name() string
	ExecPath() string
	ExecArgs(string) []string
	RenderOnChange(graylog.ResponseCollectorConfiguration, string) bool
	ValidateConfigurationFile(string) bool
}

type Creator func(*context.Ctx) Backend

type backendFactory struct {
	registry map[string]Creator
}

func (bf *backendFactory) register(name string, c Creator) error {
	if _, ok := bf.registry[name]; ok {
		log.Error("Collector backend named " + name + " is already registered")
		return nil
	}
	bf.registry[name] = c
	return nil
}

func (bf *backendFactory) get(name string) (Creator, error) {
	c, ok := bf.registry[name]
	if !ok {
		log.Fatal("No collector backend named " + name + " is registered")
		return nil, nil
	}
	return c, nil
}

// global registry
var factory = &backendFactory{registry: make(map[string]Creator)}

func RegisterBackend(name string, c Creator) error {
	return factory.register(name, c)
}

func GetBackend(name string) (Creator, error) {
	return factory.get(name)
}
