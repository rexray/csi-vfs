package service

import (
	"context"
	"net"
	"os"
	"path"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"github.com/thecodeteam/gocsi"
	"github.com/thecodeteam/gocsi/csp"
	"github.com/thecodeteam/gofsutil"
)

const (
	// Name is the name of this CSI SP.
	Name = "com.thecodeteam.vfs"

	// VendorVersion is the version of this CSP SP.
	VendorVersion = "0.1.4"

	// SupportedVersions is a list of the CSI versions this SP supports.
	SupportedVersions = "0.0.0, 0.1.0"
)

// Service is a CSI SP and gocsi.IdempotencyProvider.
type Service interface {
	csi.ControllerServer
	csi.IdentityServer
	csi.NodeServer
	gocsi.IdempotencyProvider
	BeforeServe(context.Context, *csp.StoragePlugin, net.Listener) error
}

type service struct {
	bindfs  string
	data    string
	dev     string
	mnt     string
	vol     string
	volGlob string
}

// New returns a new Service.
func New() Service {
	return &service{}
}

func (s *service) BeforeServe(
	ctx context.Context, sp *csp.StoragePlugin, lis net.Listener) error {

	defer func() {
		log.WithFields(map[string]interface{}{
			"bindfs":  s.bindfs,
			"data":    s.data,
			"dev":     s.dev,
			"mnt":     s.mnt,
			"vol":     s.vol,
			"volGlob": s.volGlob,
		}).Infof("configured %s", Name)
	}()

	if v, ok := gocsi.LookupEnv(ctx, EnvVarDataDir); ok {
		s.data = v
	}
	if s.data == "" {
		if v, ok := gocsi.LookupEnv(ctx, "HOME"); ok {
			s.data = path.Join(v, ".csi-vfs")
		} else if v, ok := gocsi.LookupEnv(ctx, "USER_PROFILE"); ok {
			s.data = path.Join(v, ".csi-vfs")
		}
	}
	if err := os.MkdirAll(s.data, 0755); err != nil {
		return err
	}
	resolveSymlink(&s.data)

	if v, ok := gocsi.LookupEnv(ctx, EnvVarDevDir); ok {
		s.dev = v
	}
	if s.dev == "" {
		s.dev = path.Join(s.data, "dev")
	}
	if err := os.MkdirAll(s.dev, 0755); err != nil {
		return err
	}
	resolveSymlink(&s.dev)

	if v, ok := gocsi.LookupEnv(ctx, EnvVarMntDir); ok {
		s.mnt = v
	}
	if s.mnt == "" {
		s.mnt = path.Join(s.data, "mnt")
	}
	if err := os.MkdirAll(s.mnt, 0755); err != nil {
		return err
	}
	resolveSymlink(&s.mnt)

	if v, ok := gocsi.LookupEnv(ctx, EnvVarVolDir); ok {
		s.vol = v
	}
	if s.vol == "" {
		s.vol = path.Join(s.data, "vol")
	}
	if err := os.MkdirAll(s.vol, 0755); err != nil {
		return err
	}
	resolveSymlink(&s.vol)

	if v, ok := gocsi.LookupEnv(ctx, EnvVarVolGlob); ok {
		s.vol = v
	}
	if s.volGlob == "" {
		s.volGlob = "*"
	}
	s.volGlob = path.Join(s.vol, s.volGlob)

	if v, ok := gocsi.LookupEnv(ctx, EnvVarBindFS); ok {
		s.bindfs = v
	}
	if s.bindfs == "" {
		s.bindfs = "bindfs"
	}

	return nil
}

// fileExists returns a flag indicating whether or not a file
// path exists.
func fileExists(filePath string) bool {
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		return true
	}
	return false
}

func resolveSymlink(symPath *string) error {
	realPath, err := filepath.EvalSymlinks(*symPath)
	if err != nil {
		return err
	}
	*symPath = realPath
	return nil
}

func getVolumeMountPaths(
	ctx context.Context, mntDir, volumeID string) ([]string, error) {

	mntPath := path.Join(mntDir, volumeID)

	minfo, err := gofsutil.GetMounts(ctx)
	if err != nil {
		return nil, err
	}

	var mountPaths []string

	for _, mi := range minfo {
		if mi.Source == mntPath {
			mountPaths = append(mountPaths, mi.Path)
		}
	}

	return mountPaths, nil
}
