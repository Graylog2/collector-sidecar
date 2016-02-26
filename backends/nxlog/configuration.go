package nxlog

import (
	"path"
	"reflect"
)

type NxConfig struct {
	CollectorPath string
	Definitions   []nxdefinition
	Paths         []nxpath
	Extensions    []nxextension
	Inputs        []nxinput
	Outputs       []nxoutput
	Routes        []nxroute
	Matches       []nxmatch
	Snippets      []nxsnippet
	Canned        []nxcanned
	Inventory     interface{}
}

type nxdefinition struct {
	name  string
	value string
}

type nxpath struct {
	name string
	path string
}

type nxextension struct {
	name       string
	properties map[string]string
}

type nxinput struct {
	name       string
	properties map[string]string
}

type nxoutput struct {
	name       string
	properties map[string]string
}

type nxroute struct {
	name       string
	properties map[string]string
}

type nxmatch struct {
	name       string
	properties map[string]string
}

type nxsnippet struct {
	name  string
	value string
}

type nxcanned struct {
	name       string
	kind       string
	properties map[string]string
}

func NewCollectorConfig(collectorPath string) *NxConfig {
	nxc := &NxConfig{
		CollectorPath: collectorPath,
		Definitions:   []nxdefinition{{name: "ROOT", value: path.Dir(collectorPath)}},
		Extensions: []nxextension{{name: "gelf", properties: map[string]string{"Module": "xm_gelf"}}},
	}
	return nxc
}

func (nxc *NxConfig) Add(class string, name string, value interface{}) {
	switch class {
	case "extension":
		addition := &nxextension{name: name, properties: value.(map[string]string)}
		nxc.Extensions = append(nxc.Extensions, *addition)
	case "input":
		addition := &nxinput{name: name, properties: value.(map[string]string)}
		nxc.Inputs = append(nxc.Inputs, *addition)
	case "output":
		addition := &nxoutput{name: name, properties: value.(map[string]string)}
		nxc.Outputs = append(nxc.Outputs, *addition)
	case "route":
		addition := &nxroute{name: name, properties: value.(map[string]string)}
		nxc.Routes = append(nxc.Routes, *addition)
	case "match":
		addition := &nxmatch{name: name, properties: value.(map[string]string)}
		nxc.Matches = append(nxc.Matches, *addition)
	case "snippet":
		addition := &nxsnippet{name: name, value: value.(string)}
		nxc.Snippets = append(nxc.Snippets, *addition)
	//pre-canned configuration types
	case "output-gelf-udp":
		addition := &nxcanned{name: name, kind: class, properties: value.(map[string]string)}
		nxc.Canned = append(nxc.Canned, *addition)
	case "input-file":
		addition := &nxcanned{name: name, kind: class, properties: value.(map[string]string)}
		nxc.Canned = append(nxc.Canned, *addition)
	case "input-windows-event-log":
		addition := &nxcanned{name: name, kind: class, properties: value.(map[string]string)}
		nxc.Canned = append(nxc.Canned, *addition)
	}
}

func (nxc *NxConfig) Update(a *NxConfig) {
	nxc.CollectorPath = a.CollectorPath
	nxc.Definitions = a.Definitions
	nxc.Paths = a.Paths
	nxc.Extensions = a.Extensions
	nxc.Inputs = a.Inputs
	nxc.Outputs = a.Outputs
	nxc.Routes = a.Routes
	nxc.Matches = a.Matches
	nxc.Snippets = a.Snippets
	nxc.Canned = a.Canned
}

func (nxc *NxConfig) Equals(a *NxConfig) bool {
	return reflect.DeepEqual(nxc, a)
}

func (nxc *NxConfig) GetCollectorPath() string {
	return nxc.CollectorPath
}

func (nxc *NxConfig) SetInventory(inventory interface{}) {
	nxc.Inventory = inventory
}
