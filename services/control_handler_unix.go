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

// +build darwin linux solaris freebsd

package services

import "github.com/kardianos/service"

func ControlHandler(action string) {
	switch action {
	case "install":
	case "uninstall":
	case "start":
	case "stop":
	case "restart":
	case "status":
	default:
		log.Fatalf("Valid service actions: %s", service.ControlAction)
	}
}
