package service

import (
	"math/rand"
	"os"
	"path"
	"path/filepath"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/akutz/gofsutil"
	"github.com/container-storage-interface/spec/lib/go/csi"
	csiutils "github.com/rexray/gocsi/utils"
)

func (s *service) CreateVolume(
	ctx context.Context,
	req *csi.CreateVolumeRequest) (
	*csi.CreateVolumeResponse, error) {

	// Get the path to the volume directory and create it if necessary.
	volPath := path.Join(s.vol, req.Name)
	if ok, err := fileExists(volPath); !ok {
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "%s: %v", volPath, err)
		}

		// Create the volume's virtual directory if it does not exist.
		if err := os.MkdirAll(volPath, 0755); err != nil {
			return nil, status.Errorf(codes.Internal, "mkdir failed: %v", err)
		}
	}

	// Get the path to the volume's info file.
	volInfoPath := path.Join(volPath, infoFileName)
	if ok, err := fileExists(volInfoPath); !ok {
		if err != nil {
			return nil, status.Errorf(
				codes.NotFound, "%s: %v", volInfoPath, err)
		}

		// Assign the volume info structure that is marshaled to disk.
		vol := volumeInfo{
			CreateVolumeRequest: *req,
			path:                volPath,
			infoPath:            volInfoPath,
		}

		// Figure out the volume's capacity.
		if cr := vol.CapacityRange; cr != nil {
			if cr.RequiredBytes == cr.LimitBytes {
				vol.capacityBytes = cr.RequiredBytes
			} else {
				// Generate a random size that is somewhere between the min
				// and max limits provided by the request.
				vol.capacityBytes = uint64(rand.Int63n(int64(cr.LimitBytes))) +
					cr.RequiredBytes
			}
		}

		// Save the volume info to disk.
		if err := vol.save(); err != nil {
			return nil, err
		}

		return &csi.CreateVolumeResponse{
			VolumeInfo: vol.toCSIVolInfo(),
		}, nil
	}

	// Create a new volumeInfo object and try to unmarshal its contents
	// from disk.
	vol := &volumeInfo{path: volPath, infoPath: volInfoPath}
	if err := vol.load(); err != nil {
		return nil, err
	}

	// Validate request capacity range against existing size.
	if cr := req.CapacityRange; cr != nil {
		if vol.capacityBytes < cr.RequiredBytes {
			return nil, status.Errorf(
				codes.AlreadyExists,
				"required bytes exceeds existing: %d", vol.capacityBytes)
		}
		if vol.capacityBytes > cr.LimitBytes {
			return nil, status.Errorf(
				codes.AlreadyExists,
				"limit bytes less than existing: %d", vol.capacityBytes)
		}
	}

	// Validate request parameters against existing parameters.
	if len(req.Parameters) != len(vol.Parameters) {
		return nil, status.Error(
			codes.AlreadyExists,
			"requested params exceed existing")
	}
	for k, v := range req.Parameters {
		if v != vol.Parameters[k] {
			return nil, status.Error(
				codes.AlreadyExists,
				"requested params != existing")
		}
	}

	// Validate request capabilities against existing capabilities.
	if ok, err := csiutils.AreVolumeCapabilitiesCompatible(
		req.VolumeCapabilities, vol.VolumeCapabilities); !ok {
		if err != nil {
			return nil, err
		}
		return nil, status.Error(
			codes.AlreadyExists,
			"requested capabilities incompatible w existing")
	}

	return &csi.CreateVolumeResponse{
		VolumeInfo: vol.toCSIVolInfo(),
	}, nil
}

func (s *service) DeleteVolume(
	ctx context.Context,
	req *csi.DeleteVolumeRequest) (
	*csi.DeleteVolumeResponse, error) {

	// Get the path of the volume.
	volPath := path.Join(s.vol, req.VolumeId)
	if _, err := fileExists(volPath); err != nil {
		return nil, status.Errorf(codes.NotFound, "%s: %v", volPath, err)
	}

	// Attempt to delete the "volume".
	if err := os.RemoveAll(volPath); err != nil {
		return nil, status.Errorf(
			codes.Internal, "delete failed: %s: %v", volPath, err)
	}

	// Indicate the operation was a success.
	return &csi.DeleteVolumeResponse{}, nil
}

func (s *service) ControllerPublishVolume(
	ctx context.Context,
	req *csi.ControllerPublishVolumeRequest) (
	*csi.ControllerPublishVolumeResponse, error) {

	// Get the existing volume info.
	vol, err := s.getVolumeInfo(req.VolumeId)
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

	// Get the path of the volume's device and see if it exists.
	devPath := path.Join(s.dev, req.VolumeId)
	ok, err := fileExists(devPath)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%s: %v", devPath, err)
	}

	// If the volume's device path already exists then check to see if
	// this is an idempotent publish.
	if !ok {
		if err := os.MkdirAll(devPath, 0755); err != nil {
			return nil, status.Errorf(
				codes.Internal, "mkdir failed: %s: %v", devPath, err)
		}
	}

	// Get the mount info to determine if the volume dir is already
	// bind mounted to the device dir.
	minfo, err := getMounts(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal, "failed to get mount info: %v", err)
	}
	mounted := false
	for _, i := range minfo {
		// If bindfs is not used then the device path will not match
		// the volume path, otherwise test both the source and target.
		if i.Source == vol.path && i.Path == devPath {
			mounted = true
			break
		}
	}

	if !mounted {
		if err := gofsutil.BindMount(ctx, vol.path, devPath); err != nil {
			return nil, status.Errorf(
				codes.Internal, "bind mount failed: src=%s, tgt=%s: %v",
				vol.path, devPath, err)
		}
	}

	return &csi.ControllerPublishVolumeResponse{
		PublishVolumeInfo: map[string]string{"path": devPath},
	}, nil
}

func (s *service) ControllerUnpublishVolume(
	ctx context.Context,
	req *csi.ControllerUnpublishVolumeRequest) (
	*csi.ControllerUnpublishVolumeResponse, error) {

	// Get the path of the volume and ensure it exists.
	volPath := path.Join(s.vol, req.VolumeId)
	if ok, err := fileExists(volPath); !ok {
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "%s: %v", volPath, err)
		}
		return nil, status.Error(codes.NotFound, volPath)
	}

	// Get the path of the volume's device.
	devPath := path.Join(s.dev, req.VolumeId)

	// Get the node's mount information.
	minfo, err := getMounts(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal, "failed to get mount info: %v", err)
	}

	// The loop below unmounts the device path if it is mounted.
	for _, i := range minfo {
		// If there is a device that matches the volPath value and
		// a path that matches the devPath value then unmount it as
		// it is the subject of this request.
		if i.Source == volPath && i.Path == devPath {
			if err := gofsutil.Unmount(ctx, devPath); err != nil {
				return nil, status.Errorf(codes.Internal,
					"failed to unmount device dir: %s: %v", devPath, err)
			}
		}
	}

	// If the device path exists then remove it.
	ok, err := fileExists(devPath)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%s: %v", devPath, err)
	}
	if ok {
		if err := os.RemoveAll(devPath); err != nil {
			return nil, status.Errorf(codes.Internal,
				"failed to remove device dir: %s: %v", devPath, err)
		}
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (s *service) ValidateVolumeCapabilities(
	ctx context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest) (
	*csi.ValidateVolumeCapabilitiesResponse, error) {

	// Get the existing volume info.
	vol, err := s.getVolumeInfo(req.VolumeId)
	if err != nil {
		return nil, err
	}

	supported := true
	msg := ""

	// Validate that the volume capabilities from the request are
	// compatible with the existing volume's capabilities.
	if ok, err := csiutils.AreVolumeCapabilitiesCompatible(
		req.VolumeCapabilities, vol.VolumeCapabilities); !ok {
		if err != nil {
			return nil, err
		}
		supported = false
		msg = "incompatible volume capabilities"
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Supported: supported,
		Message:   msg,
	}, nil
}

func (s *service) ListVolumes(
	ctx context.Context,
	req *csi.ListVolumesRequest) (
	*csi.ListVolumesResponse, error) {

	fileNames, err := filepath.Glob(s.volGlob)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to list volume dir: %s: %v", s.volGlob, err)
	}

	rep := &csi.ListVolumesResponse{
		Entries: make([]*csi.ListVolumesResponse_Entry, len(fileNames)),
	}
	for i, volInfoPath := range fileNames {
		vol := volumeInfo{
			path:     path.Dir(volInfoPath),
			infoPath: volInfoPath,
		}
		if err := vol.load(); err != nil {
			return nil, err
		}
		rep.Entries[i] = &csi.ListVolumesResponse_Entry{
			VolumeInfo: vol.toCSIVolInfo(),
		}
	}

	return rep, nil
}

func (s *service) GetCapacity(
	ctx context.Context,
	req *csi.GetCapacityRequest) (
	*csi.GetCapacityResponse, error) {

	return nil, status.Error(codes.Unimplemented, "")
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
