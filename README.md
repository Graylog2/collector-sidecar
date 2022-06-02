# Graylog Sidecar

[![Go Report Card](https://goreportcard.com/badge/Github.com/graylog2/collector-sidecar)](https://goreportcard.com/report/github.com/Graylog2/collector-sidecar)

The Graylog Sidecar is a supervisor process for 3rd party log collectors like NXLog and filebeat.
The Sidecar program is able to fetch and validate configuration files from a Graylog server for various log collectors.
You can think of it like a centralized configuration and process management system for your log collectors.

The master branch is tracking the development version of the Sidecar.

## Documentation

Please check our official [documentation](https://docs.graylog.org/docs/sidecar) for more information.

## Installation

Please check our [installation documentation](https://docs.graylog.org/docs/sidecar#installation) for more information.


## Compile

  * Clone the repository
  * Run `make` to install the dependencies and build the binary for the local platform
  * Run `make help` to see more targets
