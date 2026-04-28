PREFIX ?= /usr/local

.PHONY: build test lint clean install uninstall setup

build:
	go build -o dx .

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -f dx

install: build
	install -d $(PREFIX)/bin
	install -m 755 dx $(PREFIX)/bin/dx

uninstall:
	rm -f $(PREFIX)/bin/dx

setup:
	ln -sf ../../scripts/pre-commit .git/hooks/pre-commit
	@echo "Pre-commit hook installed."
