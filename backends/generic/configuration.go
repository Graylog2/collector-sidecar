package generic

import (
	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/context"

	"reflect"
)

type GenericConfig struct {
	Context           *context.Ctx
	UserConfig        *cfgfile.SidecarBackend
	BackendId         string
	serviceType       string
	operatingSystem   string
	executablePath    string
	configurationPath string
	executeParameters []string
	validationCommand string
	Template          string
}

func NewCollectorConfig(context *context.Ctx) *GenericConfig {
	g := &GenericConfig{
		Context: context,
	}
	return g
}

func (g *GenericConfig) Update(a *GenericConfig) {
	g.Template = a.Template
}

func (g *GenericConfig) Equals(a *GenericConfig) bool {
	return reflect.DeepEqual(g, a)
}
