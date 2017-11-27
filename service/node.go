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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/thecodeteam/gofsutil"
)

func (s *service) NodePublishVolume(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest) (
	*csi.NodePublishVolumeResponse, error) {

	// Get the path of the volume.
	devPath := path.Join(s.dev, req.VolumeId)
	mntPath := path.Join(s.mnt, req.VolumeId)
	tgtPath := req.TargetPath
	resolveSymlink(&tgtPath)

	if !fileExists(devPath) {
		log.WithField("path", devPath).Error(
			"must call ControllerPublishVolume first")
		return nil, status.Error(
			codes.Aborted, "must call ControllerPublishVolume first")
	}

	// If the private mount directory for the device does not exist then
	// create it.
	if !fileExists(mntPath) {
		if err := os.MkdirAll(mntPath, 0755); err != nil {
			log.WithField("path", mntPath).WithError(err).Error(
				"create private mount dir failed")
			return nil, err
		}
		log.WithField("path", mntPath).Info("created private mount dir")
	}

	// Get the mount info to determine if the device is already mounted
	// into the private mount directory.
	minfo, err := gofsutil.GetMounts(ctx)
	if err != nil {
		log.WithError(err).Error("failed to get mount info")
		return nil, err
	}
	isPrivMounted := false
	isTgtMounted := false
	for _, i := range minfo {
		if i.Source == devPath && i.Path == mntPath {
			isPrivMounted = true
		}
		if i.Source == mntPath && i.Path == tgtPath {
			isTgtMounted = true
		}
	}

	if isTgtMounted {
		log.WithField("path", tgtPath).Info("already mounted")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// If the devie is not already mounted into the private mount
	// area then go ahead and mount it.
	if !isPrivMounted {
		if err := gofsutil.BindMount(ctx, devPath, mntPath); err != nil {
			log.WithField("path", mntPath).WithError(err).Error(
				"create private mount failed")
			return nil, err
		}
		log.WithField("path", mntPath).Info("created private mount")
	}

	// Create the bind mount options from the requested access mode.
	var opts []string
	if vc := req.VolumeCapability; vc != nil {
		if am := req.VolumeCapability.AccessMode; am != nil {
			switch am.Mode {
			case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER:
				opts = []string{"rw"}
			case csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY:
				opts = []string{"ro"}
			default:
				return nil, status.Errorf(codes.InvalidArgument,
					"unsupported access mode: %v", am.Mode)
			}
		}
	}

	// Ensure the directory for the request's target path exists.
	if err := os.MkdirAll(tgtPath, 0755); err != nil {
		return nil, err
	}

	// Bind mount the private mount to the requested target path with
	// the requested access mode.
	if err := gofsutil.BindMount(ctx, mntPath, tgtPath, opts...); err != nil {
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (s *service) NodeUnpublishVolume(
	ctx context.Context,
	req *csi.NodeUnpublishVolumeRequest) (
	*csi.NodeUnpublishVolumeResponse, error) {

	// Get the path of the volume.
	mntPath := path.Join(s.mnt, req.VolumeId)
	tgtPath := req.TargetPath
	resolveSymlink(&tgtPath)

	// Get the node's mount information.
	minfo, err := gofsutil.GetMounts(ctx)
	if err != nil {
		log.WithError(err).Error("failed to get mount info")
		return nil, err
	}

	// The loop below does two things:
	//
	//   1. It unmounts the target path if it is mounted.
	//   2. It counts how many times the volume is mounted.
	mountCount := 0
	for _, i := range minfo {

		// If there is a device that matches the mntPath value then
		// increment the number of times this volume is mounted on
		// this node.
		if i.Source == mntPath {
			mountCount++
		}

		// If there is a device that matches the mntPath value and
		// a path that matches the tgtPath value then unmount it as
		// it is the subject of this request.
		if i.Source == mntPath && i.Path == tgtPath {
			if err := gofsutil.Unmount(ctx, tgtPath); err != nil {
				log.WithField("path", tgtPath).WithError(err).Error(
					"failed to unmount target path")
				return nil, err
			}
			log.WithField("path", tgtPath).Info("unmounted target path")
			mountCount--
		}
	}

	log.WithFields(map[string]interface{}{
		"name":  req.VolumeId,
		"count": mountCount,
	}).Info("volume mount info")

	// If the target path exists then remove it.
	if fileExists(tgtPath) {
		if err := os.RemoveAll(tgtPath); err != nil {
			log.WithField("path", tgtPath).WithError(err).Error(
				"failed to remove target path")
			return nil, err
		}
		log.WithField("path", tgtPath).Info("removed target path")
	}

	// If the volume is no longer mounted anywhere else on this node then
	// unmount the volume's private mount as well.
	if mountCount == 0 {
		if err := gofsutil.Unmount(ctx, mntPath); err != nil {
			log.WithField("path", mntPath).WithError(err).Error(
				"failed to unmount private mount")
			return nil, err
		}
		log.WithField("path", mntPath).Info("unmounted private mount")
		if err := os.RemoveAll(mntPath); err != nil {
			log.WithField("path", mntPath).WithError(err).Error(
				"failed to remove private mount")
			return nil, err
		}
		log.WithField("path", mntPath).Info("removed private mount")
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (s *service) GetNodeID(
	ctx context.Context,
	req *csi.GetNodeIDRequest) (
	*csi.GetNodeIDResponse, error) {

	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	return &csi.GetNodeIDResponse{NodeId: hostname}, nil
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
