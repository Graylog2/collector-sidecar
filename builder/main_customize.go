// Add OpenTelemetry Collector customizations to this file.
package main

import (
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/otelcol"
)

func customizeSettings(params *otelcol.CollectorSettings) {
}

func customizeCommand(params *otelcol.CollectorSettings, cmd *cobra.Command) {

}
