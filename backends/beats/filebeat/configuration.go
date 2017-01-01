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

package filebeat

import (
	"github.com/Graylog2/collector-sidecar/backends/beats"
	"github.com/Graylog2/collector-sidecar/context"
)

type FileBeatConfig struct {
	Beats *beats.BeatsConfig
}

func NewCollectorConfig(context *context.Ctx) *FileBeatConfig {
	bc := &beats.BeatsConfig{
		Context:             context,
		Container:           map[string]interface{}{},
		ContainerKeyMapping: map[string]string{"indexname": "index"},
	}
	backendIndex, err := context.UserConfig.GetBackendIndexByName(name)
	if err == nil {
		bc.UserConfig = &context.UserConfig.Backends[backendIndex]
	}
	return &FileBeatConfig{Beats: bc}
}
