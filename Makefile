BINDIR := bin
GO     := go

.PHONY: build build-tools run test fmt tidy lint clean $(BINDIR)

build: $(BINDIR)/chaosmith-mcp

$(BINDIR):
	@mkdir -p $(BINDIR)

$(BINDIR)/chaosmith-mcp: | $(BINDIR)
	$(GO) build -o $@ .

build-tools: $(BINDIR)/build-pca

$(BINDIR)/build-pca: | $(BINDIR)
	$(GO) build -o $@ ./util/embxform/cmd/build-pca

run:
	$(GO) run .

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

lint:
	$(GO) vet ./...

clean:
	$(GO) clean -cache -testcache ./...
	@if [ -d "$(BINDIR)" ]; then rm -rf "$(BINDIR)"; fi
