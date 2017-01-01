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
	"testing"

	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/cfgfile"
)

func TestFilebeatRenderTrigger(t *testing.T) {
	context := context.NewContext()
	context.UserConfig = &cfgfile.SidecarConfig{}
	backendUserConfiguration := &cfgfile.SidecarBackend{Name: "filebeat"}
	context.UserConfig.Backends = []cfgfile.SidecarBackend{*backendUserConfiguration}

	engine := NewCollectorConfig(context)
	engine.Beats.Version = []int{1, 0, 0}
	serverResponse := graylog.ResponseCollectorConfiguration{}

	triggered := engine.RenderOnChange(serverResponse)
	if triggered != true { // initially the configuration is empty, a render call create a new configuration
		t.Error("Initial render call did not trigger a new configuration file")
	}

	triggered = engine.RenderOnChange(serverResponse)
	if triggered != false { // second should not be triggered, current configuration and server response are the same
		t.Error("Second render call did trigger a new configuration. This could potentially loop forever.")
	}
}

func TestFilebeat5RenderTrigger(t *testing.T) {
	context := context.NewContext()
	context.UserConfig = &cfgfile.SidecarConfig{}
	backendUserConfiguration := &cfgfile.SidecarBackend{Name: "filebeat"}
	context.UserConfig.Backends = []cfgfile.SidecarBackend{*backendUserConfiguration}

	engine := NewCollectorConfig(context)
	engine.Beats.Version = []int{5, 0, 0}
	serverResponse := graylog.ResponseCollectorConfiguration{}

	triggered := engine.RenderOnChange(serverResponse)
	if triggered != true { // initially the configuration is empty, a render call create a new configuration
		t.Error("Initial render call did not trigger a new configuration file")
	}

	triggered = engine.RenderOnChange(serverResponse)
	if triggered != false { // second should not be triggered, current configuration and server response are the same
		t.Error("Second render call did trigger a new configuration. This could potentially loop forever.")
	}
}
