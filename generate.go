package main

// Generate the OpenTelemetry Collector binary source code.
//go:generate go tool go.opentelemetry.io/collector/cmd/builder --config ./builder/builder-config.yaml --skip-compilation
// Modify the generated main.go to add customization hooks.
//go:generate go run ./builder/mod/main.go -main-path ./builder/main.go
