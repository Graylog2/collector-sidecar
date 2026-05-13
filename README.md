# Graylog Collector

[![Go Report Card](https://goreportcard.com/badge/Github.com/graylog2/collector-sidecar)](https://goreportcard.com/report/github.com/Graylog2/collector-sidecar)

Graylog Collector is a log shipper that ships logs from your hosts to a Graylog server.
It runs and supervises an embedded [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)
and is managed remotely by Graylog using the [Open Agent Management Protocol (OpAMP)](https://opentelemetry.io/docs/specs/opamp/).
Configuration, certificates, and updates are delivered from the server after
the agent is enrolled with an enrollment endpoint and token.

The Collector is distributed as native Linux packages (deb, rpm) and a
Windows MSI installer that registers a Windows Service.

## Build

This repository uses [Task](https://taskfile.dev) as its build tool.

  * Clone the repository
  * Run `go tool task build` to build the binary for the local platform
  * Run `go tool task --list` to see all available targets
