GO ?= go

all: clean build

build:
	$(GO) build -v -i -o sidecar

build-linux:
	GOOS=linux GOARCH=amd64 $(GO) build -v -i -o sidecar

build-darwin:
	GOOS=darwin GOARCH=amd64 $(GO) build -v -i -o sidecar

build-windows:
	GOOS=windows GOARCH=amd64 $(GO) build -v -i -o sidecar.exe

misc:
	$(GO) build -o misc/nxmock/nxlog misc/nxmock/main.go

fmt:
	$(GO) fmt

clean:
	rm -f sidecar sidecar.exe 

.PHONY: all build build-darwin build-windows misc clean
