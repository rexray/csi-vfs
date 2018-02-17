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

test: build | $(ETCD) $(CSC)
	@umount /tmp/vol-00 "$(HOME)/.csi-vfs/*/vol-00" 2> /dev/null || true
	@rm -fr /tmp/vol-00
	test ! -e "$(HOME)/.csi-vfs/*/vol-00" -a ! -e /tmp/vol-00
	@echo
	@pkill $(notdir $(ETCD)) || true
	@pkill -2 $(notdir $(CSI_VFS)) || true
	@rm -fr $(X_CSI_LOG) $(CSI_ENDPOINT) etcd.log default.etcd
	$(ETCD) > etcd.log 2>&1 &
	@echo
	$(CSI_VFS) > $(X_CSI_LOG) 2>&1 &
	@echo
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
	  if grep -q "msg=serving" $(X_CSI_LOG); then break; \
	  else sleep 0.1; fi \
	done
	$(CSC) -v $(X_CSI_VERSION) i version
	@echo
	$(CSC) -v $(X_CSI_VERSION) i info
	@echo
	((cat /proc/self/mountinfo 2> /dev/null || mount) | grep vol-00) || true
	@echo
	$(CSC) -v $(X_CSI_VERSION) c new \
           --cap SINGLE_NODE_WRITER,mount,vfs \
           vol-00
	@echo
	test -e "$(HOME)/.csi-vfs/vol/vol-00"
	@echo
	$(CSC) -v $(X_CSI_VERSION) c publish \
           --cap SINGLE_NODE_WRITER,mount,vfs \
           --node-id localhost \
           vol-00
	@echo
	test -e "$(HOME)/.csi-vfs/dev/vol-00"
	@echo
	mkdir -p /tmp/vol-00
	@echo
	$(CSC) -v $(X_CSI_VERSION) n publish \
           --cap SINGLE_NODE_WRITER,mount,vfs \
           --target-path /tmp/vol-00 \
           --pub-info devPath=$(HOME)/.csi-vfs/dev/vol-00 \
           vol-00
	@echo
	test -e "$(HOME)/.csi-vfs/mnt/vol-00"
	@echo
	(cat /proc/self/mountinfo 2> /dev/null || mount) | grep vol-00
	@echo
	$(CSC) -v $(X_CSI_VERSION) n unpublish --target-path /tmp/vol-00 vol-00
	@echo
	test ! -e "$(HOME)/.csi-vfs/mnt/vol-00"
	@echo
	$(CSC) -v $(X_CSI_VERSION) c unpublish --node-id localhost vol-00
	@echo
	test ! -e "$(HOME)/.csi-vfs/dev/vol-00"
	@echo
	$(CSC) -v $(X_CSI_VERSION) c delete vol-00
	@echo
	test ! -e "$(HOME)/.csi-vfs/vol/vol-00"
	@echo
	pkill -2 $(notdir $(CSI_VFS))
	@echo
	pkill $(notdir $(ETCD))
	@echo
	((cat /proc/self/mountinfo 2> /dev/null || mount) | grep vol-00) || true
	@echo
	cat $(X_CSI_LOG)


################################################################################
##                                 BUILD                                      ##
################################################################################
build: $(CSI_VFS)

.PHONY: build test
