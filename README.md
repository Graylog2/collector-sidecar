# Graylog Sidecar

**Required Graylog version:** 2.0 and later + installed [graylog-plugin-collector](https://github.com/Graylog2/graylog-plugin-collector/blob/master/README.md)

Installation
------------

[Download the binary](https://github.com/Graylog2/sidecar/releases) and place it in a directory included in your `$PATH`.

Usage
-----

```
Usage of ./sidecar:
  -collector-conf-path string
      File path to the rendered collector configuration (default "/tmp/nxlog.conf")
  -collector-id string
      UUID used for collector registration
  -collector-path string
      Path to collector installation (default "/usr/bin/nxlog")
  -log-path string
      Path to log directory (default "/var/log/sidecar")
  -node-id string
      Collector identification string (default "graylog-sidecar")
  -server-url string
      Graylog server URL (default "http://127.0.0.1:12900")
  -service string
      Control the system service
  -tags string
      Comma separated tag list
```

Example command:

```
sidecar -collector-id bb62865b-ed41-4494-85b4-0df22890b784 -node-id nxlog-linux -collector-path /opt/nxlog/nxlog -log-path ./log -collector-conf-path ./nxlog/nxlog.conf -server-url http://localhost:12900  -tags my,tags
```

You can also use the `sidecar.ini` file to persist startup options.

Compile
-------

  * Clone the repository into your `$GOPATH` under `src/github.com/Graylog2/sidecar`
  * Install the [glide package manager](https://glide.sh)
  * run `glide install` in the sidecar directory
  * (for Go <1.6 `export GO15VENDOREXPERIMENT=1`)
  * run `make`

Development
-----------

There is a collector mock programm to use the sidecar without actually running a collector like NXLog. Simply build it with
`make misc` und use the option `-collector-path misc/nxmock/nxlog`.
