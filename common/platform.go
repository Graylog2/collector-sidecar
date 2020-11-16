// Copyright (C) 2020 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.


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
