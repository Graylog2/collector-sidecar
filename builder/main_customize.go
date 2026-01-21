// Add OpenTelemetry Collector customizations to this file.
package main

import (
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/otelcol"
	"go.uber.org/zap"
)

func customizeSettings(params *otelcol.CollectorSettings) {
	// Disable caller information in logs to reduce log chatter and avoid exposing source code file names.
	params.LoggingOptions = append(params.LoggingOptions, zap.WithCaller(false))
}

func customizeCommand(params *otelcol.CollectorSettings, cmd *cobra.Command) {

}
