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

package services

import (
	"github.com/kardianos/service"

	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func ControlHandler(action string) {
	switch action {
	case "install":
	case "uninstall":
		cleanupInstalledServices()
	case "start":
	case "stop":
	case "restart":
	case "status":
	default:
		log.Fatalf("Valid service actions: %s", service.ControlAction)
	}
}

func cleanupInstalledServices() {
	// backends are not initialized yet, let's use names directly
	uninstallService("graylog-collector-nxlog")
}

func uninstallService(name string) {
	log.Infof("Uninstalling service %s", name)
	m, err := mgr.Connect()
	if err != nil {
		log.Errorf("Failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	// service exist so we try to uninstall it
	if err != nil {
		log.Debugf("Service %s doesn't exist, no uninstall action needed", name)
		return
	}

	defer s.Close()
	err = s.Delete()
	if err != nil {
		log.Errorf("Can't delete service %s: %v", name, err)
	}

	err = eventlog.Remove(s.Name)
	if err != nil {
		log.Errorf("RemoveEventLogSource() failed: %s", err)
	}

	return
}
