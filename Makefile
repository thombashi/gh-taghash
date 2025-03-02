VERSION := v0.3.2

EXTENSION_NAME := gh-taghash
EXTENSION := thombashi/$(EXTENSION_NAME)

BIN_DIR := $(CURDIR)/bin

# build binary must be located in the root of the project to test it with the local gh
BUILD_BIN := $(EXTENSION_NAME)

TEST_FORMAT := json


GOIMPORTS := $(BIN_DIR)/goimports
$(GOIMPORTS):
	mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install golang.org/x/tools/cmd/goimports@latest

STATICCHECK := $(BIN_DIR)/staticcheck
$(STATICCHECK):
	mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install honnef.co/go/tools/cmd/staticcheck@latest

TESTIFYILINT := $(BIN_DIR)/testifylint
$(TESTIFYILINT):
	mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install github.com/Antonboom/testifylint@latest

.PHONY: build
build:
	go build -o $(BUILD_BIN) .

.PHONY: clean
clean:
	rm -rf $(BIN_DIR) $(BUILD_BIN)

.PHONY: check
check: $(STATICCHECK) $(TESTIFYILINT)
	$(STATICCHECK) ./...
	go vet ./...
	$(TESTIFYILINT) ./...

.PHONEY: fmt
fmt: $(GOIMPORTS) $(TESTIFYILINT)
	$(GOIMPORTS) -w .
	gofmt -w -s .
	$(TESTIFYILINT) -fix ./...

.PHONY: help
help: build
	$(BUILD_BIN) --help

.PHONY: uninstall
uninstall:
	-gh extension remove $(EXTENSION)

.PHONY: install
install: build uninstall
	gh extension install .
	gh extension list

.PHONEY: push-tag
push-tag:
	git push origin $(VERSION)

.PHONY: tag
tag:
	git tag $(VERSION) -m "Release $(VERSION)"

.PHONY: test
test:
	go test -v ./...

run-test: install
	gh taghash --repo actions/checkout --log-level=debug --format=$(TEST_FORMAT) \
		v1.1.0 \
		ec3afacf7f605c9fc12c70bc1c9e1708ddb99eca \
		0b496e91ec7ae4428c3ed2eeb4c3a40df431f2cc \
		a5ac7e51b41094c92402da3b24376905380afc29 \
		6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6 \
		v4.1.6-4-g6ccd57f \
		692973e3d937129bcbf40652eb9f2f61becf3332

	gh taghash --repo actions/checkout --log-level=debug --format=$(TEST_FORMAT) --show-base-tag \
		6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6

run-no-cache-test: install
	gh taghash --repo actions/checkout --log-level=debug --format=$(TEST_FORMAT) --no-cache \
		v1.1.0 \
		a5ac7e51b41094c92402da3b24376905380afc29 \
		6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6 \
		v4.1.6-4-g6ccd57f \
		692973e3d937129bcbf40652eb9f2f61becf3332

	gh taghash --repo actions/checkout --log-level=debug --format=$(TEST_FORMAT) --show-base-tag --no-cache \
		6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6
