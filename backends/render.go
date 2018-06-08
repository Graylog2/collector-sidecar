// This file is part of Graylog.
//
// Graylog is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Graylog is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Graylog.  If not, see <http://www.gnu.org/licenses/>.

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
	err = ioutil.WriteFile(b.ConfigurationPath, stringConfig, 0600)
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
