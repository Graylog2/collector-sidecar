package cfgfile

import (
	"fmt"

	"github.com/Graylog2/collector-sidecar/common"
)

func withPlatformDefaults(config *SidecarConfig) {
	config.NodeId = fmt.Sprintf("file:%s", common.ConfigBasePath("node-id"))
	config.CachePath = common.ConfigBasePath("cache")
	config.LogPath = common.ConfigBasePath("logs")
	config.CollectorConfigurationDirectory = common.ConfigBasePath("generated")
	config.CollectorBinariesAccesslist = []string{
		common.ConfigBasePath("filebeat.exe"),
		common.ConfigBasePath("winlogbeat.exe"),
		"C:\\Program Files\\Filebeat\\filebeat.exe",
		"C:\\Program Files\\Packetbeat\\packetbeat.exe",
		"C:\\Program Files\\Metricbeat\\metricbeat.exe",
		"C:\\Program Files\\Heartbeat\\heartbeat.exe",
		"C:\\Program Files\\Auditbeat\\auditbeat.exe",
		"C:\\Program Files (x86)\\nxlog\\nxlog.exe",
		"C:\\Program Files\\nxlog\\nxlog.exe",
	}
	config.WindowsDriveRange = "CDEFGHIJKLMNOPQRSTUVWXYZ"
}
