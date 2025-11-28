//go:build !windows

package cfgfile

import (
	"fmt"

	"github.com/Graylog2/collector-sidecar/common"
)

func withPlatformDefaults(config *SidecarConfig) {
	directoryName := common.LowerFullName()

	config.NodeId = fmt.Sprintf("file:%s", common.ConfigBasePath("node-id"))
	config.CachePath = fmt.Sprintf("/var/cache/%s", directoryName)
	config.LogPath = fmt.Sprintf("/var/log/%s", directoryName)
	config.CollectorConfigurationDirectory = fmt.Sprintf("/var/lib/%s/generated", directoryName)
	config.CollectorBinariesAccesslist = []string{
		"/usr/bin/filebeat",
		"/usr/bin/packetbeat",
		"/usr/bin/metricbeat",
		"/usr/bin/heartbeat",
		"/usr/bin/auditbeat",
		"/usr/bin/journalbeat",
		fmt.Sprintf("/usr/lib/%s/filebeat", directoryName),
		fmt.Sprintf("/usr/lib/%s/auditbeat", directoryName),
		"/usr/share/filebeat/bin/filebeat",
		"/usr/share/packetbeat/bin/packetbeat",
		"/usr/share/metricbeat/bin/metricbeat",
		"/usr/share/heartbeat/bin/heartbeat",
		"/usr/share/auditbeat/bin/auditbeat",
		"/usr/share/journalbeat/bin/journalbeat",
		"/usr/bin/nxlog",
		"/opt/nxlog/bin/nxlog",
	}
}
