export SHELL := $(shell env which bash)

all: build

################################################################################
##                                 HOME                                       ##
################################################################################
HOME ?= /tmp/csi-vfs
export HOME


################################################################################
##                                GOPATH                                      ##
################################################################################
# Ensure GOPATH is set and that it contains only a single element.
GOPATH ?= $(HOME)/go
GOPATH := $(word 1,$(subst :, ,$(GOPATH)))
export GOPATH


################################################################################
##                               CSI-VFS                                      ##
################################################################################
CSI_VFS := ./csi-vfs
$(CSI_VFS): *.go provider/*.go service/*.go
	go build -o $@ .


################################################################################
##                                 TEST                                       ##
################################################################################
X_CSI_VERSION ?= 0.1.0
X_CSI_LOG ?= csi-vfs.log
CSI_ENDPOINT ?= csi-vfs.sock
X_CSI_LOG_LEVEL ?= debug
X_CSI_REQ_LOGGING ?= true
X_CSI_REP_LOGGING ?= true
X_CSI_SERIAL_VOL_ACCESS_ETCD_ENDPOINTS ?= 127.0.0.1:2379

export CSI_ENDPOINT
export X_CSI_LOG_LEVEL
export X_CSI_REQ_LOGGING X_CSI_REP_LOGGING
export X_CSI_SERIAL_VOL_ACCESS_ETCD_ENDPOINTS

ETCD := ./etcd
$(ETCD): | $(CSI_VFS)
	go get -u -d github.com/coreos/etcd
	go build -o $@ github.com/coreos/etcd

CSC := ./csc
$(CSC):
	go build -o $@ ./vendor/github.com/thecodeteam/gocsi/csc

VOL_ID := vol-00
VOL_JSN := .info.json
TGT_DIR := /tmp/$(VOL_ID)
VFS_DIR := $(HOME)/.csi-vfs
VOL_DIR := $(VFS_DIR)/vol/$(VOL_ID)
DEV_DIR := $(VFS_DIR)/dev/$(VOL_ID)
MNT_DIR := $(VFS_DIR)/mnt/$(VOL_ID)

test-clean:
	@echo "CLEANUP"
	@umount $(TGT_DIR) $(VOL_DIR) $(DEV_DIR) $(MNT_DIR) 2> /dev/null || true
	@test ! "$$(mount | grep $(VOL_ID))"
	@echo "- verified volume not mounted"
	@rm -fr "$(VFS_DIR)/*/$(VOL_ID)" "$(TGT_DIR)"
	@test ! -e "$(VFS_DIR)/*/$(VOL_ID)" -a ! -e $(TGT_DIR)
	@echo "- verified volume paths do not exist"
	@pkill $(notdir $(ETCD)) || true
	@echo "- killed etcd"
	@rm -fr etcd.log default.etcd
	@echo "- removed etcd log file & data directory"
	@pkill -2 $(notdir $(CSI_VFS)) || true
	@echo "- killed csi-vfs"
	@rm -fr $(X_CSI_LOG) $(CSI_ENDPOINT)
	@echo "- removed csi-vfs log file & unix socket"
	@echo "- test environment ready"

test-up:
	@$(MAKE) --no-print-directory test-clean
	@echo
	@echo "INITIALIZATION"
	@$(ETCD) > etcd.log 2>&1 &
	@echo "- started etcd"
	@$(CSI_VFS) > $(X_CSI_LOG) 2>&1 &
	@echo "- started csi-vfs"
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
	  if grep -q "msg=serving" $(X_CSI_LOG); then break; \
	  else sleep 0.1; fi \
	done
	@echo "- csi-vfs ready"
	@echo
	@echo "GET SUPPORTED VERSIONS"
	$(CSC) -v $(X_CSI_VERSION) i version
	@echo
	@echo "GET PLUG-IN INFO"
	$(CSC) -v $(X_CSI_VERSION) i info
	@echo
	@echo "CREATE NEW VOLUME"
	$(CSC) -v $(X_CSI_VERSION) c new \
      --cap SINGLE_NODE_WRITER,mount,vfs \
      $(VOL_ID)
	@echo
	@echo "VERIFY VOLUME DIR"
	test -e "$(VOL_DIR)" -a -e "$(VOL_DIR)/$(VOL_JSN)"
	@echo
	@echo "CONTROLLER PUBLISH VOLUME"
	$(CSC) -v $(X_CSI_VERSION) c publish \
      --cap SINGLE_NODE_WRITER,mount,vfs \
      --node-id localhost \
      $(VOL_ID)
	@echo
	@echo "VERIFY DEVICE DIR"
	test -e "$(DEV_DIR)" -a -e "$(DEV_DIR)/$(VOL_JSN)"
	@echo
	@echo "VERIFY VOLUME->DEVICE BIND MOUNT"
	test "$$(mount | grep $(DEV_DIR))"
	@echo
	@echo "CREATE TARGET PATH"
	mkdir -p $(TGT_DIR)
	@echo
	@echo "NODE PUBLISH VOLUME"
	$(CSC) -v $(X_CSI_VERSION) n publish \
      --cap SINGLE_NODE_WRITER,mount,vfs \
      --target-path $(TGT_DIR) \
      --pub-info devPath=$(VFS_DIR)/dev/$(VOL_ID) \
      $(VOL_ID)
	@echo
	@echo "VERIFY MOUNT DIR"
	test -e "$(MNT_DIR)" -a -e "$(MNT_DIR)/$(VOL_JSN)"
	@echo
	@echo "VERIFY DEVICE->MOUNT BIND MOUNT"
	test "$$(mount | grep $(MNT_DIR))"
	@echo
	@echo "VERIFY MOUNT->TARGET BIND MOUNT"
	test "$$(mount | grep $(TGT_DIR))"
	@echo
	@echo "NODE PUBLISH VOLUME (IDEMPONTENT)"
	$(CSC) -v $(X_CSI_VERSION) n publish \
      --cap SINGLE_NODE_WRITER,mount,vfs \
      --target-path $(TGT_DIR) \
      --pub-info devPath=$(VFS_DIR)/dev/$(VOL_ID) \
      $(VOL_ID)
	@echo
	@echo "VERIFY SINGLE MOUNT->TARGET BIND MOUNT"
	test "1" -eq "$$(mount | grep $(TGT_DIR) | wc -l | awk '{print $$1}')"

test-down:
	@echo "NODE UNPUBLISH VOLUME"
	$(CSC) -v $(X_CSI_VERSION) n unpublish --target-path $(TGT_DIR) $(VOL_ID)
	@echo
	@echo "VERIFY ! MOUNT->TARGET BIND MOUNT"
	test ! "$$(mount | grep $(TGT_DIR))"
	@echo
	@echo "VERIFY ! DEVICE->MOUNT BIND MOUNT"
	test ! "$$(mount | grep $(MNT_DIR))"
	@echo
	@echo "VERIFY ! MOUNT DIR"
	test ! -e "$(MNT_DIR)"
	@echo
	@echo "CONTROLLER UNPUBLISH VOLUME"
	$(CSC) -v $(X_CSI_VERSION) c unpublish --node-id localhost $(VOL_ID)
	@echo
	@echo "VERIFY ! VOLUME->DEVICE BIND MOUNT"
	test ! "$$(mount | grep $(DEV_DIR))"
	@echo
	@echo "VERIFY ! DEVICE DIR"
	test ! -e "$(DEV_DIR)"
	@echo
	@echo "CONTROLLER DELETE VOLUME"
	$(CSC) -v $(X_CSI_VERSION) c delete $(VOL_ID)
	@echo
	@echo "VERIFY ! VOLUME DIR"
	test ! -e "$(VOL_DIR)"
	@echo
	@$(MAKE) --no-print-directory test-clean 1> /dev/null

test: build | $(ETCD) $(CSC)
	@$(MAKE) --no-print-directory test-up
	@echo
	@$(MAKE) --no-print-directory test-down

docker-test:
	docker run --privileged --rm -it \
           -v $(shell pwd):/go/src/github.com/thecodeteam/csi-vfs golang:1.9.4 \
           make -C /go/src/github.com/thecodeteam/csi-vfs test


################################################################################
##                                 BUILD                                      ##
################################################################################
build: $(CSI_VFS)

clean:
	rm -fr $(CSI_VFS) $(ETCD) $(CSC)

.PHONY: build clean test test-clean docker-test
