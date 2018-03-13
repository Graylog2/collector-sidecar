package generic

import (
	"os/exec"
	"github.com/Graylog2/collector-sidecar/common"
	"io/ioutil"
	"github.com/Graylog2/collector-sidecar/api/graylog"
	"bytes"
)

func (g *GenericConfig) Render() []byte {
	var result bytes.Buffer
	result.WriteString(g.Template)

	return common.ConvertLineBreak(result.Bytes())
}

func (g *GenericConfig) RenderToFile() error {
	stringConfig := g.Render()
	err := common.CreatePathToFile(g.ConfigurationPath())
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(g.ConfigurationPath(), stringConfig, 0644)
	return err
}

func (g *GenericConfig) RenderOnChange(json graylog.ResponseCollectorConfiguration) bool {
	configFromResponse := NewCollectorConfig(g.Context)
	for _, snippet := range json.Snippets {
		if snippet.Backend == g.BackendId {
			configFromResponse.Template = snippet.Value
		}
	}

	if !g.Equals(configFromResponse) {
		log.Infof("[%s] Configuration change detected, rewriting configuration file.", g.Name())
		g.Update(configFromResponse)
		g.RenderToFile()
		return true
	}

	return false
}

func (g *GenericConfig) ValidateConfigurationFile() bool {
	output, err := exec.Command(g.validationCommand).CombinedOutput()
	soutput := string(output)
	if err != nil {
		log.Errorf("[%s] Error during configuration validation: %s", g.Name(), soutput)
		return false
	}

	return true
}

