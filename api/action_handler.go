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

package api

import (
	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/api/rest"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/daemon"
)

func HandleCollectorActions(actions []graylog.ResponseCollectorAction, ctx *context.Ctx) {
	for _, action := range actions {
		switch {
		case action.Properties["restart"] == true:
			restartAction(action)
		case action.Properties["import"] == true:
			configurationImportAction(action, ctx)
		}
	}
}

func restartAction(action graylog.ResponseCollectorAction) {
	for name, runner := range daemon.Daemon.Runner {
		if name == action.Backend {
			log.Infof("[%s] Executing requested collector restart", name)
			runner.Restart()
		}
	}
}

func configurationImportAction(action graylog.ResponseCollectorAction, ctx *context.Ctx) {
	for name := range daemon.Daemon.Runner {
		if name == action.Backend {
			log.Infof("[%s] Sending configuration to Graylog server", name)
			backend := backends.Store.GetBackend(name)
			renderedConfiguration := backend.RenderToString()
			httpClient := rest.NewHTTPClient(GetTlsConfig(ctx))
			UploadConfiguration(httpClient, ctx,
				&graylog.CollectorUpload{
					CollectorId: ctx.CollectorId,
					NodeId: ctx.NodeId,
					CollectorName: backend.Name(),
					RenderedConfiguration: renderedConfiguration})

		}
	}
}