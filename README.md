# Graylog Collector Sidecar

**Required Graylog version:** 2.0 and later + installed [graylog-plugin-collector](https://github.com/Graylog2/graylog-plugin-collector/blob/master/README.md)

Installation
------------

[Download the binary](https://github.com/Graylog2/collector-sidecar/releases) and place it in a directory included in your `$PATH`.

Install
-------

**Ubuntu**
Install the NXLog package from the offical download [page](https://nxlog.org/products/nxlog-community-edition/download)

```
  $ /etc/init.d/nxlog stop
  $ update-rc.d -f nxlog remove
  $ gpasswd -a nxlog adm
  $ install -d -o nxlog -g nxlog /var/run/nxlog
 
  $ cp graylog-collector-sidecar /usr/bin/
  $ mkdir -p /var/log/graylog/collector-sidecar
  $ mkdir -p /etc/graylog/collector-sidecar/generated
  $ cp collector_sidecar.ini /etc/graylog/collectorr-sidecar/
```

Edit `/etc/graylog/collector-sidecar/collector\_sidecar.ini`, you should set at least the correct URL to your Graylog server and proper tags.
The tags are used to define which configurations the host should receive.

```
  $ graylog_collector_sidecar -service install
  $ start collector_sidecar
```

**Windows**
Install the NXLog package from the offical download [page](https://nxlog.org/products/nxlog-community-edition/download)

```
  $ C:\Program Files (x86)\nxlog\nxlog -u

  $ mkdir C:\Program Files (x86)\graylog\collector-sidecar\generated
  $ cp graylog_collector_sidecar.exe C:\Program Files (x86)\graylog\collector-sidecar\
  $ cp collector_sidecar_windows.ini C:\Program Files (x86)\graylog\collector-sidecar\collector_sidecar.ini
  $ C:\Program Files (x86)\graylog\collector-sidecar\graylog_collector_sidecar.exe -service install
```

Edit `C:\Program Files (x86)\graylog\collector-sidecar\collector\_sidecar.ini`, you should set at least the correct URL to your Graylog server and proper tags.

```
  $ C:\Program Files (x86)\graylog\collector-sidecar\graylog_collector_sidecar.exe -service restart
```

Compile
-------

  * Clone the repository into your `$GOPATH` under `src/github.com/Graylog2/collector-sidecar`
  * Install the [glide package manager](https://glide.sh)
  * run `glide install` in the collector-sidecar directory
  * (for Go <1.6 `export GO15VENDOREXPERIMENT=1`)
  * run `make`

Development
-----------

There is a collector mock programm to use the collector-sidecar without actually running a collector like NXLog. Simply build it with
`make misc` und use the option `-collector-path misc/nxmock/nxlog`.
