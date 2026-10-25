.PHONY: build frontend check install uninstall clean
.DEFAULT_GOAL := build

BIN ?= fly

# 优先装到已在 PATH 且可写的目录（Homebrew），否则退回 GOPATH/bin
DEST :=
ifneq ($(wildcard /opt/homebrew/bin),)
ifeq ($(shell test -w /opt/homebrew/bin && echo y),y)
DEST := /opt/homebrew/bin
endif
endif
ifeq ($(DEST),)
DEST := $(shell go env GOPATH)/bin
endif

frontend:
	cd web && npm ci --no-fund --no-audit && npm run build

check:
	cd web && npm ci --no-fund --no-audit && npm test && npm run typecheck
	go test -race ./...
	go vet ./...

build:
	go build -o $(BIN) .

install: build
	install -d "$(DEST)"
	install -m 755 $(BIN) "$(DEST)/$(BIN)"
	@echo "已安装到 $(DEST)/$(BIN)"

uninstall:
	rm -f "$(DEST)/$(BIN)"
	@echo "已卸载 $(DEST)/$(BIN)"

clean:
	rm -f $(BIN)
