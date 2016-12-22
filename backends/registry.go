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
	"fmt"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/system"
	"github.com/Graylog2/collector-sidecar/logger"
)

var (
	log = logger.Log()
	// global registry
	factory = &backendFactory{registry: make(map[string]Creator)}
	Store   = &backendStore{backends: make(map[string]Backend)}
)

type Backend interface {
	Name() string
	Driver() string
	ExecPath() string
	ConfigurationPath() string
	ExecArgs() []string
	RenderOnChange(graylog.ResponseCollectorConfiguration) bool
	ValidateConfigurationFile() bool
	ValidatePreconditions() bool
	Status() system.Status
	SetStatus(int, string)
}

const (
	StatusRunning int = 0
	StatusUnknown int = 1
	StatusError   int = 2
)

type Creator func(*context.Ctx) Backend

type backendFactory struct {
	registry map[string]Creator
}

type backendStore struct {
	backends map[string]Backend
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

func RegisterBackend(name string, c Creator) error {
	return factory.register(name, c)
}

func GetCreator(name string) (Creator, error) {
	return factory.get(name)
}

func SetStatusLogErrorf(name string, format string, args ...interface{}) error {
	Store.backends[name].SetStatus(StatusError, fmt.Sprintf(format, args...))
	log.Errorf(fmt.Sprintf("[%s] ", name)+format, args...)
	return fmt.Errorf(format, args)
}

func (bs *backendStore) AddBackend(backend Backend) {
	bs.backends[backend.Name()] = backend
}

func (bs *backendStore) GetBackend(name string) Backend {
	return bs.backends[name]
}
