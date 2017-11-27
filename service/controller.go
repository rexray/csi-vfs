package service

import (
	"os"
	"path"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/thecodeteam/gofsutil"
)

func (s *service) CreateVolume(
	ctx context.Context,
	req *csi.CreateVolumeRequest) (
	*csi.CreateVolumeResponse, error) {

	// Get the path of the volume.
	volPath := path.Join(s.vol, req.Name)

	// Create the volume's virtual directory if it does not exist.
	if err := os.MkdirAll(volPath, 0755); err != nil {
		return nil, err
	}

	log.WithField("volPath", volPath).Info("created new volume")
	return &csi.CreateVolumeResponse{
		VolumeInfo: &csi.VolumeInfo{Id: req.Name},
	}, nil
}

func (s *service) DeleteVolume(
	ctx context.Context,
	req *csi.DeleteVolumeRequest) (
	*csi.DeleteVolumeResponse, error) {

	// Get the path of the volume.
	volPath := path.Join(s.vol, req.VolumeId)

	// Attempt to delete the "device".
	if err := os.RemoveAll(volPath); err != nil {
		log.WithField("path", volPath).WithError(err).Error(
			"delete directory failed")
		return nil, err
	}

	// Indicate the operation was a success.
	log.WithField("volPath", volPath).Info("deleted volume")
	return &csi.DeleteVolumeResponse{}, nil
}

func (s *service) ControllerPublishVolume(
	ctx context.Context,
	req *csi.ControllerPublishVolumeRequest) (
	*csi.ControllerPublishVolumeResponse, error) {

	// Get the path of the volume.
	volPath := path.Join(s.vol, req.VolumeId)

	// Get the path of the volume's device.
	devPath := path.Join(s.dev, req.VolumeId)

	// If the private mount directory for the device does not exist then
	// create it.
	if !fileExists(devPath) {
		if err := os.MkdirAll(devPath, 0755); err != nil {
			log.WithField("path", devPath).WithError(err).Error(
				"create device dir failed")
			return nil, err
		}
		log.WithField("path", devPath).Info("created device dir")
	}

	// Get the mount info to determine if the volume dir is already
	// bind mounted to the device dir.
	minfo, err := gofsutil.GetMounts(ctx)
	if err != nil {
		log.WithError(err).Error("failed to get mount info")
		return nil, err
	}
	mounted := false
	for _, i := range minfo {
		// If bindfs is not used then the device path will not match
		// the volume path, otherwise test both the source and target.
		if i.Source == volPath && i.Path == devPath {
			mounted = true
			break
		}
	}

	if mounted {
		log.WithField("path", devPath).Info("already bind mounted")
	} else {
		if err := gofsutil.BindMount(ctx, volPath, devPath); err != nil {
			log.WithField("name", req.VolumeId).WithError(err).Error(
				"bind mount failed")
			return nil, err
		}
		log.WithField("path", devPath).Info("bind mounted volume to device")
	}

	return &csi.ControllerPublishVolumeResponse{
		PublishVolumeInfo: map[string]string{"path": devPath},
	}, nil
}

func (s *service) ControllerUnpublishVolume(
	ctx context.Context,
	req *csi.ControllerUnpublishVolumeRequest) (
	*csi.ControllerUnpublishVolumeResponse, error) {

	// Get the path of the volume.
	volPath := path.Join(s.vol, req.VolumeId)

	// Get the path of the volume's device.
	devPath := path.Join(s.dev, req.VolumeId)

	// Get the node's mount information.
	minfo, err := gofsutil.GetMounts(ctx)
	if err != nil {
		log.WithError(err).Error("failed to get mount info")
		return nil, err
	}

	// The loop below unmounts the device path if it is mounted.
	for _, i := range minfo {
		// If there is a device that matches the volPath value and
		// a path that matches the devPath value then unmount it as
		// it is the subject of this request.
		if i.Source == volPath && i.Path == devPath {
			if err := gofsutil.Unmount(ctx, devPath); err != nil {
				log.WithField("path", devPath).WithError(err).Error(
					"failed to unmount device dir")
				return nil, err
			}
		}
	}

	// If the device path exists then remove it.
	if fileExists(devPath) {
		if err := os.RemoveAll(devPath); err != nil {
			log.WithField("path", devPath).WithError(err).Error(
				"failed to remove device dir")
			return nil, err
		}
	}

	log.WithField("path", devPath).Info("unmount success")
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (s *service) ValidateVolumeCapabilities(
	ctx context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest) (
	*csi.ValidateVolumeCapabilitiesResponse, error) {

	// If any of the requested capabilities are related to block
	// devices then indicate lack of support.
	supported := true
	message := ""
	for _, vc := range req.VolumeCapabilities {
		if vc.GetBlock() != nil {
			supported = false
			message = "raw device access is not supported"
			break
		}
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Supported: supported,
		Message:   message,
	}, nil
}

func (s *service) ListVolumes(
	ctx context.Context,
	req *csi.ListVolumesRequest) (
	*csi.ListVolumesResponse, error) {

	fileNames, err := filepath.Glob(s.volGlob)
	if err != nil {
		return nil, err
	}

	rep := &csi.ListVolumesResponse{
		Entries: make([]*csi.ListVolumesResponse_Entry, len(fileNames)),
	}
	for i, volPath := range fileNames {
		volName := path.Base(volPath)
		rep.Entries[i] = &csi.ListVolumesResponse_Entry{
			VolumeInfo: &csi.VolumeInfo{Id: volName},
		}
	}

	return rep, nil
}

func (s *service) GetCapacity(
	ctx context.Context,
	req *csi.GetCapacityRequest) (
	*csi.GetCapacityResponse, error) {

	return &csi.GetCapacityResponse{
		AvailableCapacity: getAvailableBytes(s.dev),
	}, nil
}

func (s *service) ControllerGetCapabilities(
	ctx context.Context,
	req *csi.ControllerGetCapabilitiesRequest) (
	*csi.ControllerGetCapabilitiesResponse, error) {

	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
					},
				},
			},
		},
	}, nil
}

func (s *service) ControllerProbe(
	ctx context.Context,
	req *csi.ControllerProbeRequest) (
	*csi.ControllerProbeResponse, error) {

	return &csi.ControllerProbeResponse{}, nil
}
