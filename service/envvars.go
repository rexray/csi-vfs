package service

const (
	// EnvVarBindFS is the name of the environment variable
	// used to obtain the path to the `bindfs` binary -- a
	// program used to provide bind mounting via FUSE on
	// operating systems that do not natively support bind
	// mounts.
	//
	// If no value is specified and `bindfs` is required, ex.
	// darwin, then `bindfs` is looked up via the path.
	EnvVarBindFS = "X_CSI_VFS_BINDFS"

	// EnvVarDataDir is the name of the environment variable
	// used to obtain the path to the VFS plug-in's data directory.
	//
	// If not specified, the directory defaults to `$HOME/.csi-vfs`.
	EnvVarDataDir = "X_CSI_VFS_DATA"

	// EnvVarDevDir is the name of the environment variable
	// used to obtain the path to the VFS plug-in's `dev` directory.
	//
	// If not specified, the directory defaults to `$X_CSI_VFS_DATA/dev`.
	EnvVarDevDir = "X_CSI_VFS_DEV"

	// EnvVarMntDir is the name of the environment variable
	// used to obtain the path to the VFS plug-in's `mnt` directory.
	//
	// If not specified, the directory defaults to `$X_CSI_VFS_DATA/mnt`.
	EnvVarMntDir = "X_CSI_VFS_MNT"

	// EnvVarVolDir is the name of the environment variable
	// used to obtain the path to the VFS plug-in's `vol` directory.
	//
	// If not specified, the directory defaults to `$X_CSI_VFS_DATA/vol`.
	EnvVarVolDir = "X_CSI_VFS_VOL"

	// EnvVarVolGlob is the name of the environment variable
	// used to obtain the glob pattern used to list the files inside
	// the $X_CSI_VFS_VOL directory. Matching files are considered
	// volumes.
	//
	// If not specified, the glob pattern defaults to `*`.
	//
	// Valid patterns are documented at
	// https://golang.org/pkg/path/filepath/#Match.
	EnvVarVolGlob = "X_CSI_VFS_VOL_GLOB"
)
