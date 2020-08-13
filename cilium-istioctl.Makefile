$(shell mkdir -p out)
$(shell $(shell pwd)/common/scripts/setup_env.sh envfile > out/.env)
include out/.env
# An export free of arugments in a Makefile places all variables in the Makefile into the
# environment. This behavior may be surprising to many that use shell often, which simply
# displays the existing environment
export

export VERSION = $(shell grep "ENV VERSION " Dockerfile | cut -d ' ' -f 3)
BUILD_DIR := $(TARGET_OUT)/release
RELEASE_DIR := cilium-istioctl-$(VERSION)

RELEASE_TARGETS := linux-amd64.tar.gz linux-arm64.tar.gz osx.tar.gz osx-arm64.tar.gz win.zip
RELEASE_FILES := $(foreach target,$(RELEASE_TARGETS),$(RELEASE_DIR)/cilium-istioctl-$(VERSION)-$(target))
RELEASE_FILES += $(foreach archive,$(RELEASE_FILES),$(archive).sha256)

all: build $(RELEASE_DIR) $(RELEASE_FILES)

$(RELEASE_DIR):
	mkdir -p $@
	-rm $@/cilium-istioctl*

%.sha256: %
	cd $(dir $<) && sha256sum $(notdir $<) > $(notdir $@)

$(RELEASE_DIR)/cilium-istioctl-$(VERSION)-win.zip: $(BUILD_DIR)/istioctl-win.exe
	cp $< cilium-istioctl.exe
	-rm $@
	zip $@ cilium-istioctl.exe
	rm cilium-istioctl.exe

$(RELEASE_DIR)/cilium-istioctl-$(VERSION)-%.tar.gz: $(BUILD_DIR)/istioctl-%
	cp $< cilium-istioctl
	tar cvzf $@ cilium-istioctl
	rm cilium-istioctl

.PHONY: build
build:
	-rm $(BUILD_DIR)/istioctl*
	BUILD_WITH_CONTAINER=1 $(MAKE) istioctl-all
