package beats

import (
	"testing"

	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/cfgfile"
)

// Beats 1.x:
//
//	output:
//	  logstash:
//	    hosts:
//	    - 1.1.1.1:5044
//	    loadbalance: true
//	    tls:
//	      certificate: /ssl/cert
//	      certificate_authorities:
//	      - /ssl/ca
//	      certificate_key: /ssl/key
//	      insecure: true
//	shipper:
//	  tags:
//	  - windows
//	  - iis
//	winlogbeat:
//	  event_logs:
//	  - name: Application
//	  - name: System
//	  - name: Security`

// Beats 5.x
//
//	`output:
//	  logstash:
//	    hosts:
//	    - 1.1.1.1:5044
//	    loadbalance: true
//	    ssl:
//	      certificate: /ssl/cert
//	      certificate_authorities:
//	      - /ssl/ca
//	      key: /ssl/key
//	      verification_mode: none
//	path:
//	  data: /beats/data
//	  logs: /var/logs
//	tags:
//	- windows
//	- iis
//	winlogbeat:
//	  event_logs:
//	  - name: Application
//	  - name: System
//	  - name: Security`

func TestBeats5MigrationSSL(t *testing.T) {
	bc := &BeatsConfig{
		Context: context.NewContext(),
		Container:  map[string]interface{}{},
		UserConfig: &cfgfile.SidecarBackend{},
		Version:    []int{5, 0, 0},
	}
	bc.UserConfig.BinaryPath = "/beats/winlogbeat.exe"
	bc.Context.UserConfig = &cfgfile.SidecarConfig{LogPath: "/var/logs"}

	bc.Set("/ssl/key", "output", "logstash", "tls", "certificate_key")
	bc.Set(true, "output", "logstash", "tls", "insecure")

	bc.RunMigrations("/beats/data")

	if bc.Get("output", "logstash", "tls") != nil {
		t.Error("'tls' key should not exist")
	}
	if ssl, ok := bc.Get("output", "logstash", "ssl").(map[string]interface{}); ok {
		if _, ok := ssl["key"]; !ok {
			t.Error("ssl key does not exist")
		}
		if _, ok := ssl["verification_mode"]; !ok {
			t.Error("verification_mode does not exist")
		}
	} else {
		t.Fail()
	}
}

func TestBeats5MigrationShipper(t *testing.T) {
	bc := &BeatsConfig{
		Context: context.NewContext(),
		Container:  map[string]interface{}{},
		UserConfig: &cfgfile.SidecarBackend{},
		Version:    []int{5, 0, 0},
	}
	bc.UserConfig.BinaryPath = "/beats/winlogbeat.exe"
	bc.Context.UserConfig = &cfgfile.SidecarConfig{LogPath: "/var/logs"}

	bc.Set([]string{"linux"}, "shipper", "tags")

	bc.RunMigrations("/beats/data")

	if bc.Get("shipper") != nil {
		t.Error("shipper key does exist")
	}
	if tags, ok := bc.Get("tags").([]string); ok {
		if tags[0] != "linux" {
			t.Error("linux tag does not exist")
		}
	} else {
		t.Error("tags does not exist")
	}
}

func TestBeats5MigrationPaths(t *testing.T) {
	bc := &BeatsConfig{
		Context: context.NewContext(),
		Container:  map[string]interface{}{},
		UserConfig: &cfgfile.SidecarBackend{},
		Version:    []int{5, 0, 0},
	}
	bc.UserConfig.BinaryPath = "/beats/winlogbeat.exe"
	bc.Context.UserConfig = &cfgfile.SidecarConfig{LogPath: "/var/logs"}

	bc.RunMigrations("/beats/data")

	if path, ok := bc.Get("path").(map[string]interface{}); ok {
		if path["data"] != "/beats/data" {
			t.Errorf("data key is wrong: %s (should be %s)", path["data"], "/beats/data")
		}
		if path["logs"] != "/var/logs" {
			t.Errorf("logs key is wrong: %s (should be %s)", path["logs"], "/var/logs")
		}
	}
}
