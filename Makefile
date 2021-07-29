NF = smf
BUILD_PATH = build
BIN_PATH = bin
CFG_PATH = config

PWD_PATH = $(shell pwd)
NF_GO_FILES = $(shell find . -name "*.go" ! -name "*_test.go")
NF_MAIN_FILE = cmd/$(NF).go
NF_CFG_FILE = $(NF)cfg.yaml

VERSION = $(shell git describe --tags)
BUILD_TIME = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_HASH = $(shell git log --pretty="%H" -1 | cut -c1-8)
COMMIT_TIME = $(shell git log --pretty="%ai" -1 | awk '{time=$$(1)"T"$$(2)"Z"; print time}')
LDFLAGS = -X bitbucket.org/free5gc-team/version.VERSION=$(VERSION) \
          -X bitbucket.org/free5gc-team/version.BUILD_TIME=$(BUILD_TIME) \
          -X bitbucket.org/free5gc-team/version.COMMIT_HASH=$(COMMIT_HASH) \
          -X bitbucket.org/free5gc-team/version.COMMIT_TIME=$(COMMIT_TIME)

.PHONY: $(NF) clean

.DEFAULT_GOAL: nf

nf: $(NF)

all: $(NF) config

$(NF): $(BUILD_PATH)/$(BIN_PATH)/$(NF)

$(BUILD_PATH)/$(BIN_PATH)/$(NF): $(NF_MAIN_FILE) $(NF_GO_FILES)
	@echo "Start building $(NF)...."
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $@ $(NF_MAIN_FILE)

config: $(BUILD_PATH)/$(CFG_PATH)/$(NF_CFG_FILE)

$(BUILD_PATH)/$(CFG_PATH)/$(NF_CFG_FILE): $(CFG_PATH)/$(NF_CFG_FILE)
	@echo "Start building $(NF_CFG_FILE)...."
	mkdir -p $(BUILD_PATH)/$(CFG_PATH)
	cp -rf $(CFG_PATH) $(BUILD_PATH)/

clean:
	rm -rf $(BUILD_PATH)
