BIN := hledit
DIST := dist
LOCAL_BIN ?= $(HOME)/.local/bin

.PHONY: build install test vet check clean

build:
	mkdir -p $(DIST)
	go build -o $(DIST)/$(BIN) .

install: build
	mkdir -p $(LOCAL_BIN)
	ln -sf $(PWD)/$(DIST)/$(BIN) $(LOCAL_BIN)/$(BIN)
	@echo "Installed $(BIN) -> $(LOCAL_BIN)/$(BIN)"

test:
	go test ./...

vet:
	go vet ./...

check: test vet
	go test -cover ./...

clean:
	rm -rf $(DIST)
