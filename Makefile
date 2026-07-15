.PHONY: build artifacts test test-integration test-hardware run fmt vet clean bridge-remote bridge-tunnel bridge-local bridge-stop openapi sdk sdk-verify sdk-test ui-deps ui-dev ui-build ui-verify android-build android-test

GO        ?= go
PKGS      ?= ./...
LDFLAGS   ?= -ldflags "-X main.Version=$(shell git rev-parse --short HEAD 2>/dev/null || echo dev)"

REMOTE    ?= root@203.0.113.10
RTTY      ?= /dev/ttyUSB3
RPORT     ?= 9300
LPTY      ?= /tmp/sim7600

build: ui-build
	$(GO) build $(LDFLAGS) -o build/sim7600d ./cmd/sim7600d

artifacts:
	./scripts/build-artifacts.sh

test:
	$(GO) test $(PKGS)

test-integration:
	$(GO) test -tags integration $(PKGS)

test-hardware:
	$(GO) test -tags hardware $(PKGS)

fmt:
	$(GO) fmt $(PKGS)

vet:
	$(GO) vet $(PKGS)

run: build
	./build/sim7600d --tty $(LPTY) --bind 127.0.0.1:8080 --db $(PWD)/sim7600d.db --at-trace

clean:
	rm -rf build/ sim7600d.db sim7600d.db-* auth_token

# Run on the remote NixOS host (one-shot foreground process; backgrounds with &).
bridge-remote:
	ssh $(REMOTE) 'socat TCP-LISTEN:$(RPORT),bind=127.0.0.1,reuseaddr,fork \
	                FILE:$(RTTY),nonblock,raw,echo=0'

# Forward the remote TCP port to localhost via SSH.
bridge-tunnel:
	ssh -N -L $(RPORT):127.0.0.1:$(RPORT) $(REMOTE)

# Terminate the local TCP into a PTY device file.
bridge-local:
	socat PTY,link=$(LPTY),raw,echo=0,mode=600 TCP:127.0.0.1:$(RPORT)

bridge-stop:
	-pkill -f "socat.*$(LPTY)"
	-pkill -f "ssh -N -L $(RPORT)"

# OpenAPI / SDK targets

openapi:
	$(GO) run ./cmd/openapi-dump > internal/api/openapi.json

sdk: openapi
	npm --prefix web/client install --no-audit --no-fund
	npm --prefix web/client run gen
	npm --prefix web/client run typecheck

sdk-verify: sdk
	@git diff --exit-code internal/api/openapi.json web/client/src \
	  || (echo "openapi.json or generated SDK is out of date; commit the changes." && exit 1)

sdk-test:
	bash web/client/test/fixtures/run.sh

# Web UI

ui-deps:
	npm --prefix web/client install --no-audit --no-fund
	npm --prefix web/ui install --no-audit --no-fund

ui-dev: ui-deps
	npm --prefix web/ui run dev

ui-build: ui-deps
	npm --prefix web/ui run build

ui-verify: ui-deps
	npm --prefix web/ui run typecheck
	npm --prefix web/ui run test

# Native Android app

android-build:
	cd android && ./gradlew assembleDebug

android-test:
	cd android && ./gradlew testDebugUnitTest lintDebug
