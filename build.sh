#!/bin/bash

GOPATH=~/Repositories/projects/go

go get github.com/kardianos/service
go get golang.org/x/sys/windows
go get gopkg.in/jmcvetta/napping.v3
go get github.com/rakyll/globalconf

rm -f main main.exe
env GOOS=windows GOARCH=amd64 go build -v -i -o gxlog.exe
env GOOS=darwin GOARCH=amd64 go build -v -i -o gxlog
