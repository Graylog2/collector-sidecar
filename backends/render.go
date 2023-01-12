// Copyright (C) 2020 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.

package backends

import (
	"bytes"
	"fmt"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/helpers"
	"io/ioutil"
)

func (b *Backend) render() []byte {
	var result bytes.Buffer
	result.WriteString(b.Template)

	return helpers.ConvertLineBreak(result.Bytes())
}

func (b *Backend) renderToFile(context *context.Ctx) error {
	if !b.CheckConfigPathAgainstAccesslist(context) {
		err := fmt.Errorf("Configuration path violates `collector_binaries_accesslist' config option.")
		b.SetStatusLogErrorf(err.Error())
		return err
	}
	stringConfig := b.render()
	err := common.CreatePathToFile(b.ConfigurationPath)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(b.ConfigurationPath, stringConfig, 0600)
	return err
}

func (b *Backend) RenderOnChange(changedBackend Backend, context *context.Ctx) bool {
	if b.Template != changedBackend.Template {
		log.Infof("[%s] Configuration change detected, rewriting configuration file.", b.Name)
		b.Template = changedBackend.Template
		if err := b.renderToFile(context); err != nil {
			return false
		}
		return true
	}
	return false
}
