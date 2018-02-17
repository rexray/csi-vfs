package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path"

	"github.com/golang/protobuf/jsonpb"
	log "github.com/sirupsen/logrus"
	"github.com/thecodeteam/gofsutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/thecodeteam/gocsi"
	csictx "github.com/thecodeteam/gocsi/context"
)

const (
	// Name is the name of this CSI SP.
	Name = "com.thecodeteam.vfs"

	// VendorVersion is the version of this CSP SP.
	VendorVersion = "0.2.1"

	// SupportedVersions is a list of the CSI versions this SP supports.
	SupportedVersions = "0.0.0, 0.1.0"

	infoFileName = ".info.json"
)

// Service is a CSI SP and gocsi.IdempotencyProvider.
type Service interface {
	csi.ControllerServer
	csi.IdentityServer
	csi.NodeServer
	BeforeServe(context.Context, *gocsi.StoragePlugin, net.Listener) error
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
	ctx context.Context, sp *gocsi.StoragePlugin, lis net.Listener) error {

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

	if v, ok := csictx.LookupEnv(ctx, EnvVarDataDir); ok {
		s.data = v
	}
	if s.data == "" {
		if v, _ := csictx.LookupEnv(ctx, "HOME"); v != "" {
			s.data = path.Join(v, ".csi-vfs")
		} else if v, _ := csictx.LookupEnv(ctx, "USER_PROFILE"); v != "" {
			s.data = path.Join(v, ".csi-vfs")
		}
	}
	if err := os.MkdirAll(s.data, 0755); err != nil {
		return err
	}
	if err := gofsutil.EvalSymlinks(ctx, &s.data); err != nil {
		return err
	}

	if v, ok := csictx.LookupEnv(ctx, EnvVarDevDir); ok {
		s.dev = v
	}
	if s.dev == "" {
		s.dev = path.Join(s.data, "dev")
	}
	if err := os.MkdirAll(s.dev, 0755); err != nil {
		return err
	}
	if err := gofsutil.EvalSymlinks(ctx, &s.dev); err != nil {
		return err
	}

	if v, ok := csictx.LookupEnv(ctx, EnvVarMntDir); ok {
		s.mnt = v
	}
	if s.mnt == "" {
		s.mnt = path.Join(s.data, "mnt")
	}
	if err := os.MkdirAll(s.mnt, 0755); err != nil {
		return err
	}
	if err := gofsutil.EvalSymlinks(ctx, &s.mnt); err != nil {
		return err
	}

	if v, ok := csictx.LookupEnv(ctx, EnvVarVolDir); ok {
		s.vol = v
	}
	if s.vol == "" {
		s.vol = path.Join(s.data, "vol")
	}
	if err := os.MkdirAll(s.vol, 0755); err != nil {
		return err
	}

	if err := gofsutil.EvalSymlinks(ctx, &s.vol); err != nil {
		return err
	}

	if v, ok := csictx.LookupEnv(ctx, EnvVarVolGlob); ok {
		s.vol = v
	}
	if s.volGlob == "" {
		s.volGlob = "*"
	}
	s.volGlob = path.Join(s.vol, s.volGlob, infoFileName)

	if v, ok := csictx.LookupEnv(ctx, EnvVarBindFS); ok {
		s.bindfs = v
	}
	if s.bindfs == "" {
		s.bindfs = "bindfs"
	}

	// Add an interceptor that validates all requests that include
	// one or more volume capabilities:
	//
	// * CreateVolume
	// * ControllerPublishVolume
	// * ValidateVolumeCapabilities
	// * NodePublishVolume
	sp.Interceptors = append(sp.Interceptors, s.validateVolumeCapabilities)

	return nil
}

type volumeInfo struct {
	csi.CreateVolumeRequest
	capacityBytes uint64
	path          string
	infoPath      string
}

func (v *volumeInfo) toCSIVolInfo() *csi.VolumeInfo {
	return &csi.VolumeInfo{
		Id:            v.Name,
		CapacityBytes: v.capacityBytes,
		Attributes:    v.Parameters,
	}
}

func (v *volumeInfo) MarshalJSON() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := &jsonpb.Marshaler{}
	if err := enc.Marshal(buf, &v.CreateVolumeRequest); err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to marshal create request: %v", err)
	}
	return json.Marshal(struct {
		CapacityBytes uint64          `json:"capacity_bytes"`
		CreateRequest json.RawMessage `json:"create_request"`
	}{
		CapacityBytes: v.capacityBytes,
		CreateRequest: buf.Bytes(),
	})
}

func (v *volumeInfo) UnmarshalJSON(data []byte) error {
	obj := struct {
		CapacityBytes uint64          `json:"capacity_bytes"`
		CreateRequest json.RawMessage `json:"create_request"`
	}{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return status.Errorf(codes.Internal,
			"failed to unmarshal volume: %v", err)
	}
	rdr := bytes.NewReader(obj.CreateRequest)
	if err := jsonpb.Unmarshal(rdr, &v.CreateVolumeRequest); err != nil {
		return status.Errorf(codes.Internal,
			"failed to unmarshal create request: %v", err)
	}
	v.capacityBytes = obj.CapacityBytes
	return nil
}

func (v *volumeInfo) save() error {
	if v.infoPath == "" {
		return status.Error(codes.Internal,
			"failed to create volume info file: empty path")
	}
	f, err := os.Create(v.infoPath)
	if err != nil {
		return status.Errorf(codes.Internal,
			"failed to create volume info file: %s: %v", v.infoPath, err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(&v)
}

func (v *volumeInfo) load() error {
	if v.infoPath == "" {
		return status.Error(codes.Internal,
			"failed to load volume info file: empty path")
	}
	f, err := os.Open(v.infoPath)
	if err != nil {
		return status.Errorf(codes.Internal,
			"failed to open volume info file: %s: %v", v.infoPath, err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	return dec.Decode(&v)
}

func (s *service) getVolumeInfo(idOrName string) (*volumeInfo, error) {

	// Get the path of the volume and ensure it exists.
	volPath := path.Join(s.vol, idOrName)
	if ok, err := fileExists(volPath); !ok {
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "%s: %v", volPath, err)
		}
		return nil, status.Error(codes.NotFound, volPath)
	}

	// Get the path of the volume info file and ensure it exists.
	volInfoPath := path.Join(volPath, infoFileName)
	if ok, err := fileExists(volInfoPath); !ok {
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "%s: %v", volInfoPath, err)
		}
		return nil, status.Error(codes.NotFound, volInfoPath)
	}

	// Create a new volumeInfo object and try to unmarshal its contents
	// from disk.
	vol := &volumeInfo{path: volPath, infoPath: volInfoPath}
	if err := vol.load(); err != nil {
		return nil, err
	}

	return vol, nil
}

// fileExists returns a flag indicating whether or not a file
// path exists.
func fileExists(filePath string) (bool, error) {
	_, err := os.Stat(filePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
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
