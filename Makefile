# Simple Makefile for common dev tasks
.PHONY: build cross-build package docker-build tidy docker run clean test

BIN := ./bin
DIST := ./dist
PKG := trinity-cache
LICENSE := LICENSE
README := README.md

# Default build (local)
build:
	mkdir -p $(BIN)
	go build -o $(BIN)/trinity-cache ./cmd/trinity-cache

# Cross-build statically linked linux binaries for release
# Usage: make cross-build VERSION=v1.2.3
cross-build:
	mkdir -p $(DIST)
	if [ -z "$(VERSION)" ]; then echo "VERSION is required (eg VERSION=v1.2.3)"; exit 1; fi
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X github.com/tommahs/trinity-cache/internal/version.Version=$(VERSION)" -o $(DIST)/$(PKG)_linux-amd64 ./cmd/trinity-cache
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64  go build -ldflags "-s -w -X github.com/tommahs/trinity-cache/internal/version.Version=$(VERSION)" -o $(DIST)/$(PKG)_linux-arm64  ./cmd/trinity-cache

# Package built binaries into tar.gz archives containing: binary, LICENSE, README.md
# Usage: make package VERSION=v1.2.3
package: cross-build
	if [ -z "$(VERSION)" ]; then echo "VERSION is required (eg VERSION=v1.2.3)"; exit 1; fi
	mkdir -p $(DIST)/tmp
	for arch in linux-amd64 linux-arm64 ; do \
		binfile=$(DIST)/$(PKG)_$${arch}; \
		if [ ! -f "$${binfile}" ]; then echo "missing binary: $${binfile}"; exit 1; fi; \
		tmpdir=$(DIST)/tmp/$${arch}; rm -rf $${tmpdir}; mkdir -p $${tmpdir}; \
		cp "$${binfile}" $${tmpdir}/$(PKG); chmod +x $${tmpdir}/$(PKG); \
		cp $(LICENSE) $${tmpdir}/ || true; cp $(README) $${tmpdir}/ || true; \
		tar -C $${tmpdir} -czf $(DIST)/$(PKG)_$(VERSION)_$${arch}.tar.gz $(PKG) $(notdir $(LICENSE)) $(notdir $(README)); \
		echo "created: $(DIST)/$(PKG)_$(VERSION)_$${arch}.tar.gz"; \
	done
	# cleanup
	rm -rf $(DIST)/tmp

# Build and push Docker image using Dockerfile.
# Requires OWNER and VERSION (VERSION may be prefixed with 'v')
# Usage: make docker-build OWNER=<owner> VERSION=v1.2.3
docker-build:
	if [ -z "$(OWNER)" ]; then echo "OWNER is required (eg OWNER=your-org)"; exit 1; fi
	if [ -z "$(VERSION)" ]; then echo "VERSION is required (eg VERSION=v1.2.3)"; exit 1; fi
	V_NO_V=$$(echo $(VERSION) | sed 's/^v//'); \
	MAJOR=$$(echo $$V_NO_V | cut -d. -f1); \
	MAJOR_MINOR=$$(echo $$V_NO_V | cut -d. -f1,2); \
	TAGS="ghcr.io/$(OWNER)/$(PKG):latest ghcr.io/$(OWNER)/$(PKG):$$V_NO_V ghcr.io/$(OWNER)/$(PKG):$$MAJOR ghcr.io/$(OWNER)/$(PKG):$$MAJOR_MINOR"; \
	echo "building image with tags: $$TAGS"; \
	docker build --pull -t ghcr.io/$(OWNER)/$(PKG):$$V_NO_V -t ghcr.io/$(OWNER)/$(PKG):$$MAJOR -t ghcr.io/$(OWNER)/$(PKG):$$MAJOR_MINOR -t ghcr.io/$(OWNER)/$(PKG):latest .; \
	echo "pushing tags..."; \
	for tag in $$V_NO_V $$MAJOR $$MAJOR_MINOR latest; do \
		docker push ghcr.io/$(OWNER)/$(PKG):$$tag; \
	done

test:
	go test -v ./...

tidy:
	go mod tidy

docker:
	docker build -t trinity-cache:dev .

run: build
	$(BIN)/trinity-cache --version || true

clean:
	rm -rf $(BIN) $(DIST)
