package cfgfile

type SidecarConfig struct {
	ServerUrl   string `config:"server_url"`
	NodeId      string `config:"node_id"`
	CollectorId string `config:"collector_id"`
	Tags 	    []string `config:"tags"`
	LogPath     string `config:"log_path"`
	Backends    struct {
			    Nxlog   struct {
					    Enabled           *bool  `config:"enabled"`
					    BinaryPath        string `config:"binary_path"`
					    ConfigurationPath string `config:"configuration_path"`
				    }
			    Topbeat struct {
					    Enabled           *bool  `config:"enabled"`
					    BinaryPath        string `config:"binary_path"`
					    ConfigurationPath string `config:"configuration_path"`
				    }
		    }
}

