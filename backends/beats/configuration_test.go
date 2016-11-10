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

	bc.RunMigrations()

	if bc.Get("output", "logstash", "tls") != nil {
		t.Fail()
	}
	if ssl, ok := bc.Get("output", "logstash", "ssl").(map[string]interface{}); ok {
		if _, ok := ssl["key"]; !ok {
			t.Fail()
		}
		if _, ok := ssl["verification_mode"]; !ok {
			t.Fail()
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

	bc.RunMigrations()

	if bc.Get("shipper") != nil {
		t.Fail()
	}
	if tags, ok := bc.Get("tags").([]string); ok {
		if tags[0] != "linux" {
			t.Fail()
		}
	} else {
		t.Fail()
	}
}

func TestBeats5MigrationPathes(t *testing.T) {
	bc := &BeatsConfig{
		Context: context.NewContext(),
		Container:  map[string]interface{}{},
		UserConfig: &cfgfile.SidecarBackend{},
		Version:    []int{5, 0, 0},
	}
	bc.UserConfig.BinaryPath = "/beats/winlogbeat.exe"
	bc.Context.UserConfig = &cfgfile.SidecarConfig{LogPath: "/var/logs"}

	bc.RunMigrations()

	if path, ok := bc.Get("path").(map[string]interface{}); ok {
		if path["data"] != "/beats/data" || path["logs"] != "/var/logs" {
			t.Fail()
		}
	}
}
