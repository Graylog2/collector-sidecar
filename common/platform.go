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