.PHONY: install test

# Same as hoptrace: clone → make install → `apilens` works from any repo.
install:
	go build -ldflags="-s -w" -o bin/apilens ./cmd/apilens
	mkdir -p "$(HOME)/.local/bin"
	ln -sf "$(CURDIR)/bin/apilens" "$(HOME)/.local/bin/apilens"
	@echo "Installed: $(HOME)/.local/bin/apilens"
	@echo "Try: apilens version   (from stance_dashboard or anywhere)"

test:
	go test ./...
