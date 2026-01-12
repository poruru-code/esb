.PHONY: build install test dev

BINARY_NAME=esb
INSTALL_DIR=$(HOME)/.local/bin
TARGET=$(INSTALL_DIR)/$(BINARY_NAME)

build:
	go build -o $(BINARY_NAME) ./cmd/esb

install: build
	@mkdir -p $(INSTALL_DIR)
	@# Move existing binary to avoid "Text file busy" if it's currently running
	@if [ -f $(TARGET) ]; then mv $(TARGET) $(TARGET).old; fi
	cp $(BINARY_NAME) $(TARGET)
	@rm -f $(TARGET).old
	@echo "✓ Installed to $(TARGET)"

test:
	go test ./...

dev:
	air
