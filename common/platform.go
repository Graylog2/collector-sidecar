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

package common

func LinuxPlatformFamily() string {
	platform := "generic"

	if FileExists("/etc/debian_version") == nil {
		platform = "debian"
	} else if FileExists("/etc/redhat-release") == nil {
		platform = "redhat"
	} else if FileExists("/etc/system-release") == nil {
		platform = "redhat"
	} else if FileExists("/etc/gentoo-release") == nil {
		platform = "gentoo"
	} else if FileExists("/etc/SuSE-release") == nil {
		platform = "suse"
	} else if FileExists("/etc/slackware-version") == nil {
		platform = "slackware"
	} else if FileExists("/etc/arch-release") == nil {
		platform = "arch"
	} else if FileExists("/etc/alpine-release") == nil {
		platform = "alpine"
	}

	return platform
}
