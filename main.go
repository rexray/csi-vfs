package main

import (
	"context"

	"github.com/thecodeteam/gocsi/csp"

	"github.com/thecodeteam/csi-vfs/provider"
	"github.com/thecodeteam/csi-vfs/service"
)

// main is ignored when this package is built as a go plug-in
func main() {
	csp.Run(
		context.Background(),
		service.Name,
		"A Virtual Filesystem (VFS) Container Storage Interface (CSI) "+
			"Storage Plug-in (SP)",
		usage,
		provider.New())
}

const usage = `    X_CSI_VFS_BINDFS
        Specifies the path to bindfs, a program that provides bind mounting
        via FUSE on operating systems that do not natively support bind
        mounts.

        The default value is bindfs.

    X_CSI_VFS_DATA
        The path to the SP's data directory.

        The default value is $HOME/.csi-vfs.

    X_CSI_VFS_DEV
        The path to the SP's device directory.

        The default value is $X_CSI_VFS_DATA/dev.

    X_CSI_VFS_MNT
        The path to the SP's mount directory.

        The default value is $X_CSI_VFS_DATA/mnt.

    X_CSI_VFS_VOL
        The path to the SP's volume directory.

        The default value is $X_CSI_VFS_DATA/vol.

    X_CSI_VFS_VOL_GLOB
        The file glob pattern used to list the files inside the
        directory $X_CSI_VFS_VOL. Matching files are considered
        volumes.

        The default value is *.
`
