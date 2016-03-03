# Graylog Sidecar

**Required Graylog version:** 2.0 and later + installed [graylog-plugin-collector](https://github.com/Graylog2/graylog-plugin-collector/blob/master/README.md)

Installation
------------

[Download the binary](https://github.com/Graylog2/sidecar/releases) and place it in a directory included in your `$PATH`.

Install
-------

**Ubuntu**
Install the NXLog package from the offical download [page](https://nxlog.org/products/nxlog-community-edition/download)

```
  $ /etc/init.d/nxlog stop
  $ update-rc.d -f nxlog remove
  $ gpasswd -a nxlog adm
  $ install -d -o nxlog -g nxlog /var/run/nxlog
 
  $ cp sidecar /usr/bin/
  $ mkdir /var/log/sidecar
  $ mkdir -p /etc/sidecar/generated
  $ cp sidecar.ini /etc/sidecar/
```

Edit `/etc/sidecar/sidecar.ini`, you should set at least the correct URL to your Graylog server and proper tags.
The tags are used to define which configurations the host should receive.

```
  $ sidecar -service install
  $ start sidecar
```

**Windows**
Install the NXLog package from the offical download [page](https://nxlog.org/products/nxlog-community-edition/download)

```
  $ C:\Program Files (x86)\nxlog\nxlog -u

  $ mkdir C:\Program Files (x86)\sidecar
  $ mkdir C:\Program Files (x86)\sidecar\generated
  $ cp sidecar.exe C:\Program Files (x86)\sidecar\
  $ cp sidecar_windows.ini C:\Program Files (x86)\sidecar\sidecar.ini
  $ C:\Program Files (x86)\sidecar\sidecar.exe -service install
```

Edit `C:\Program Files (x86)\sidecar\sidecar.ini`, you should set at least the correct URL to your Graylog server and proper tags.

```
  $ C:\Program Files (x86)\sidecar\sidecar.exe -service restart
```

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
