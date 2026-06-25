VERSION := $(shell sed -n -e 's/version:[ "]*\([^"]*\).*/\1/p' plugin.yaml)
DIST := $(CURDIR)/_dist
LDFLAGS := -X github.com/ThalesGroup/helm-spray/v4/cmd.version=$(VERSION)
GOFLAGS := -trimpath

.PHONY: dist dist_darwin dist_linux dist_windows package clean

dist: dist_darwin dist_linux dist_windows

dist_darwin:
	$(MAKE) package GOOS=darwin GOARCH=amd64 BIN=helm-spray
	$(MAKE) package GOOS=darwin GOARCH=arm64 BIN=helm-spray

dist_linux:
	$(MAKE) package GOOS=linux GOARCH=amd64 BIN=helm-spray
	$(MAKE) package GOOS=linux GOARCH=arm64 BIN=helm-spray

dist_windows:
	$(MAKE) package GOOS=windows GOARCH=amd64 BIN=helm-spray.exe

# package builds a single GOOS/GOARCH binary and archives it. Dependencies come
# from go.mod and are not mutated at build time.
package:
	mkdir -p $(DIST) bin
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build $(GOFLAGS) -o bin/$(BIN) -ldflags "$(LDFLAGS)" .
	tar -czf $(DIST)/helm-spray-$(GOOS)-$(GOARCH).tar.gz bin/$(BIN) README.md LICENSE plugin.yaml
	rm -f bin/$(BIN)

clean:
	rm -rf $(DIST) bin
