# Graylog Collector Sidecar

**Required Graylog version:** 2.0 and later + installed [graylog-plugin-collector](https://github.com/Graylog2/graylog-plugin-collector/blob/master/README.md)

Installation
------------

[Download a package](https://github.com/Graylog2/collector-sidecar/releases) and install it on the target system.


**Ubuntu**
Install the NXLog package from the offical download [page](https://nxlog.org/products/nxlog-community-edition/download)

```
  $ /etc/init.d/nxlog stop
  $ update-rc.d -f nxlog remove
  $ gpasswd -a nxlog adm
 
  $ dpkg -i collector-sidecar_0.0.1-1_amd64.deb
```

Edit `/etc/graylog/collector-sidecar/collector_sidecar.ini`, you should set at least the correct URL to your Graylog server and proper tags.
The tags are used to define which configurations the host should receive.

```
  $ graylog-collector-sidecar -service install
  $ start collector-sidecar
```

**Windows**
Install the NXLog package from the offical download [page](https://nxlog.org/products/nxlog-community-edition/download)

```
  $ C:\Program Files (x86)\nxlog\nxlog -u

  $ graylog_collector_sidecar_installer.exe
```

Edit `C:\Program Files (x86)\graylog\collector-sidecar\collector_sidecar.ini`, you should set at least the correct URL to your Graylog server and proper tags.

```
  $ C:\Program Files (x86)\graylog\collector-sidecar\graylog-collector-sidecar.exe -service install
  $ C:\Program Files (x86)\graylog\collector-sidecar\graylog-collector-sidecar.exe -service start
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
