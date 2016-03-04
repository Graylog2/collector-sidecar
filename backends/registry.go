// This file is part of Graylog.
//
// Graylog is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Graylog is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Graylog.  If not, see <http://www.gnu.org/licenses/>.

package backends

import (
	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/util"
)

var log = util.Log()

type Backend interface {
	Name() string
	ExecPath() string
	ExecArgs(string) []string
	RenderOnChange(graylog.ResponseCollectorConfiguration, string) bool
	ValidateConfigurationFile(string) bool
	ValidatePreconditions() bool
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
