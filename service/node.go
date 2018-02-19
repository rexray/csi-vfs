package service

import (
	"os"
	"os/exec"
	"path"
	"runtime"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/akutz/gofsutil"
	"github.com/container-storage-interface/spec/lib/go/csi"
	csiutils "github.com/rexray/gocsi/utils"
)

func (s *service) NodePublishVolume(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest) (
	*csi.NodePublishVolumeResponse, error) {

	// Get the existing volume info.
	vol, err := s.getVolume(req.VolumeId)
	if err != nil {
		return nil, err
	}

	// Verify that the requested capability is compatible with the volume's
	// capabilities.
	if ok, err := csiutils.IsVolumeCapabilityCompatible(
		req.VolumeCapability, vol.VolumeCapabilities); !ok {
		if err != nil {
			return nil, err
		}
		return nil, status.Error(
			codes.InvalidArgument, "invalid volume capability")
	}

	// Get the path of the volume.
	devPath := path.Join(s.dev, req.VolumeId)
	mntPath := path.Join(s.mnt, req.VolumeId)

	// Eval any symlinks in the target path and ensure the CO has created it.
	tgtPath := req.TargetPath
	if err := gofsutil.EvalSymlinks(ctx, &tgtPath); err != nil {
		return nil, status.Errorf(
			codes.Internal, "failed to eval symlink: %s: %v", tgtPath, err)
	}
	if ok, err := fileExists(tgtPath); !ok {
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "%s: %v", tgtPath, err)
		}
		return nil, status.Error(codes.NotFound, tgtPath)
	}

	// Get the path of the volume's device and see if it exists.
	if ok, err := fileExists(devPath); !ok {
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "%s: %v", devPath, err)
		}
		return nil, status.Error(
			codes.Aborted, "must call ControllerPublishVolume first")
	}

	// If the private mount directory for the device does not exist then
	// create it.
	if ok, err := fileExists(mntPath); !ok {
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "%s: %v", mntPath, err)
		}
		if err := os.MkdirAll(mntPath, 0755); err != nil {
			return nil, status.Errorf(codes.Internal,
				"create private mount dir failed: %s: %v", mntPath, err)
		}
	}

	// Get the mount info to determine if the device is already mounted
	// into the private mount directory.
	minfo, err := getMounts(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal, "failed to get mount info: %v", err)
	}
	isPrivMounted := false
	for _, i := range minfo {
		if i.Source == vol.path && i.Path == tgtPath {
			return &csi.NodePublishVolumeResponse{}, nil
		}
		if i.Source == vol.path && i.Path == mntPath {
			isPrivMounted = true
		}
	}

	// If the devie is not already mounted into the private mount
	// area then go ahead and mount it.
	if !isPrivMounted {
		if err := gofsutil.BindMount(ctx, devPath, mntPath); err != nil {
			return nil, status.Errorf(codes.Internal,
				"bind mount failed: devPath=%s, mntPath=%s: %v",
				devPath, mntPath, err)
		}
	}

	// Create the bind mount options from the requet's ReadOnly field
	// and access mode.
	opts := []string{"rw"}
	if am := req.VolumeCapability.AccessMode; req.Readonly || (am != nil &&
		am.Mode == csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY) {
		opts[0] = "ro"
	}

	// Bind mount the private mount to the requested target path with
	// the requested access mode.
	if err := gofsutil.BindMount(ctx, mntPath, tgtPath, opts...); err != nil {
		return nil, status.Errorf(codes.Internal,
			"bind mount failed: mntPath=%s, tgtPath=%s, opts=%v: %v",
			mntPath, tgtPath, opts, err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (s *service) NodeUnpublishVolume(
	ctx context.Context,
	req *csi.NodeUnpublishVolumeRequest) (
	*csi.NodeUnpublishVolumeResponse, error) {

	// Get the path of the volume and ensure it exists.
	volPath := path.Join(s.vol, req.VolumeId)
	if ok, err := fileExists(volPath); !ok {
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "%s: %v", volPath, err)
		}
		return nil, status.Error(codes.NotFound, volPath)
	}

	// Get the path of the volume.
	devPath := path.Join(s.dev, req.VolumeId)
	mntPath := path.Join(s.mnt, req.VolumeId)
	tgtPath := req.TargetPath
	if err := gofsutil.EvalSymlinks(ctx, &tgtPath); err != nil {
		return nil, status.Errorf(
			codes.Internal, "failed to eval symlink: %s: %v", tgtPath, err)
	}

	// Get the node's mount information.
	minfo, err := getMounts(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal, "failed to get mount info: %v", err)
	}

	// The loop below does two things:
	//
	//   1. It unmounts the target path if it is mounted.
	//   2. It counts how many times the volume is mounted.
	mountCount := 0
	for _, i := range minfo {

		// If there is an entry that matches the volPath value that
		// isn't the dev or mnt paths then increment the number of
		// times this volume is mounted on this node.
		if i.Source == volPath && (i.Path != devPath && i.Path != mntPath) {
			mountCount++
		}

		// If there is an entry that matches the volPath value and
		// a path that matches the tgtPath value then unmount it as
		// it is the subject of this request.
		if i.Source == volPath && i.Path == tgtPath {
			if err := gofsutil.Unmount(ctx, tgtPath); err != nil {
				return nil, status.Errorf(
					codes.Internal, "unmount failed: %s: %v", tgtPath, err)
			}
			mountCount--
		}
	}

	log.WithFields(map[string]interface{}{
		"name":  req.VolumeId,
		"count": mountCount,
	}).Debug("volume mount info")

	// If the volume is no longer mounted anywhere else on this node then
	// unmount the volume's private mount as well.
	if mountCount == 0 {
		if err := gofsutil.Unmount(ctx, mntPath); err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"unmount private mnt failed: %s: %v", mntPath, err)
		}
		if err := os.RemoveAll(mntPath); err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"remove private mnt failed: %s: %v", mntPath, err)
		}
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (s *service) NodeGetId(
	ctx context.Context,
	req *csi.NodeGetIdRequest) (
	*csi.NodeGetIdResponse, error) {

	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	return &csi.NodeGetIdResponse{NodeId: hostname}, nil
}

func (s *service) NodeProbe(
	ctx context.Context,
	req *csi.NodeProbeRequest) (
	*csi.NodeProbeResponse, error) {

	switch runtime.GOOS {
	case "linux":
		break
	case "darwin":
		if _, err := exec.LookPath(s.bindfs); err != nil {
			return nil, status.Error(codes.FailedPrecondition, s.bindfs)
		}
	default:
		return nil, status.Errorf(codes.FailedPrecondition,
			"unsupported operating system: %s", runtime.GOOS)
	}

	return &csi.NodeProbeResponse{}, nil
}

func (s *service) NodeGetCapabilities(
	ctx context.Context,
	req *csi.NodeGetCapabilitiesRequest) (
	*csi.NodeGetCapabilitiesResponse, error) {

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			&csi.NodeServiceCapability{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
					},
				},
			},
		},
	}, nil
}
