VERSION := v0.0.1

EXTENSION_NAME := gh-taghash
EXTENSION := thombashi/$(EXTENSION_NAME)

BIN_DIR := $(CURDIR)/bin

STATICCHECK := $(BIN_DIR)/staticcheck
TESTIFYILINT := $(BIN_DIR)/testifylint

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

$(STATICCHECK): $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install honnef.co/go/tools/cmd/staticcheck@latest

$(TESTIFYILINT): $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install github.com/Antonboom/testifylint@latest

.PHONY: build
build:
	go build -o $(EXTENSION_NAME) .

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)

.PHONY: check
check: $(STATICCHECK) $(TESTIFYILINT)
	$(STATICCHECK) ./...
	go vet ./...
	$(TESTIFYILINT) ./...

.PHONEY: fmt
fmt: $(TESTIFYILINT)
	gofmt -w -s .
	$(TESTIFYILINT) -fix ./...

.PHONY: help
help: build
	./$(EXTENSION_NAME) --help

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
	gh taghash --repo actions/checkout --log-level=debug \
		v1.1.0 \
		a5ac7e51b41094c92402da3b24376905380afc29 \
		6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6 \
		v4.1.6-4-g6ccd57f \
		692973e3d937129bcbf40652eb9f2f61becf3332

	gh taghash --repo actions/checkout --log-level=debug --show-base-tag \
		6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6

run-no-cache-test: install
	gh taghash --repo actions/checkout --log-level=debug --no-cache \
		v1.1.0 \
		a5ac7e51b41094c92402da3b24376905380afc29 \
		6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6 \
		v4.1.6-4-g6ccd57f \
		692973e3d937129bcbf40652eb9f2f61becf3332

	gh taghash --repo actions/checkout --log-level=debug --show-base-tag \
		6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6
