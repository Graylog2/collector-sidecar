GO ?= go

all: clean build

build:
	$(GO) build -v -i -o gxlog

build-darwin:
	GOOS=darwin GOARCH=amd64 $(GO) build -v -i -o gxlog

build-windows:
	GOOS=windows GOARCH=amd64 $(GO) build -v -i -o gxlog.exe

misc:
	$(GO) build -o misc/nxmock/nxlog misc/nxmock/main.go

clean:
	rm -f main main.exe gxlog gxlog.exe 

.PHONY: all build build-darwin build-windows misc clean
