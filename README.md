# CSI-VFS
CSI-VFS is a Container Storage Interface
([CSI](https://github.com/container-storage-interface/spec)) plug-in
that provides virtual filesystem (VFS) support.

This project may be compiled as a stand-alone binary using Golang that,
when run, provides a valid CSI endpoint. This project can also be
vendored or built as a Golang plug-in in order to extend the functionality
of other programs.

## Installation
CSI-VFS can be installed with Go and the following command:

```bash
$ go get github.com/codedellemc/csi-vfs
```

The resulting binary will be installed to `$GOPATH/bin/csi-vfs`.

## Starting the Plug-in
Before starting the plug-in please set the environment variable
`CSI_ENDPOINT` to a valid Go network address. such as `unix:///tmp/csi.sock`
or `tcp://127.0.0.1.8080`. After doing so the plug-in can be launched
with a simple command:

```bash
$ csi-vfs
INFO[0000] serving                                       address="unix:///tmp/csi.sock" service=csi-vfs
```

The server can be shutdown by using `Ctrl-C` or sending the process
any of the standard exit signals.

## Using the Plug-in
The CSI specification uses the gRPC protocol for plug-in communication.
The easiest way to interact with a CSI plug-in is via the Container
Storage Client (`csc`) program provided via the
[GoCSI](https://github.com/codedellemc/gocsi) project:

```bash
$ go get github.com/codedellemc/gocsi
$ go install github.com/codedellemc/gocsi/csc
```

## Configuring the Plug-in
The VFS plug-in attempts to approximate the normal workflow of a storage platform
by having separate directories for volumes, devices, and private mounts. These
directories can be configured with the following environment variables:

| Name | Default | Description |
|------|---------|-------------|
| `CSI_VFS_DATA` | `$HOME/.csi-vfs` | The root data directory |
| `CSI_VFS_VOL` | `$CSI_VFS_DATA/vol` | Where volumes (directories) are created |
| `CSI_VFS_VOL_GLOB` | `*` | The pattern used to match volumes in `$CSI_VFS_VOL` |
| `CSI_VFS_DEV` | `$CSI_VFS_DATA/dev` | A directory from `$CSI_VFS_VOL` is bind mounted to an eponymous directory in this location when `ControllerPublishVolume` is called |
| `CSI_VFS_MNT` | `$CSI_VFS_DATA/mnt` | A directory from `$CSI_VFS_DEV` is bind mounted to an eponymous directory in this location when `NodePublishVolume` is called |
