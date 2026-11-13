APP        := mf971v-dashboard
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    := -s -w -X main.version=$(VERSION)
GOFLAGS    := -trimpath

.PHONY: all build arm test vet fmt clean

all: test arm

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(APP) ./cmd/mf971v-dashboard

# Target used by releases: 32-bit ARMv7, statically linked, no libc dependency.
arm:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' \
		-o dist/$(APP)-linux-armv7 ./cmd/mf971v-dashboard

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -rf dist $(APP)
