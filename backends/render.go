package backends

import (
	"bytes"
	"io/ioutil"

	"github.com/Graylog2/collector-sidecar/common"
)

func (b *Backend) render() []byte {
	var result bytes.Buffer
	result.WriteString(b.Template)

	return common.ConvertLineBreak(result.Bytes())
}

func (b *Backend) renderToFile() error {
	stringConfig := b.render()
	err := common.CreatePathToFile(b.ConfigurationPath)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(b.ConfigurationPath, stringConfig, 0644)
	return err
}

func (b *Backend) RenderOnChange(changedBackend Backend) bool {
	if b.Template != changedBackend.Template {
		log.Infof("[%s] Configuration change detected, rewriting configuration file.", b.Name)
		b.Template = changedBackend.Template
		b.renderToFile()
		return true
	}

	return false
}
