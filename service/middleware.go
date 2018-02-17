package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

type hasVolumeCapability interface {
	GetVolumeCapability() *csi.VolumeCapability
}
type hasVolumeCapabilities interface {
	GetVolumeCapabilities() []*csi.VolumeCapability
}

// validateVolumeCapabilities validates the volume capabilities provided
// with request messages that have function signatures
// "GetVolumeCapability() *csi.VolumeCapability" and
// "GetVolumeCapabilities() []*csi.VolumeCapability".
func (s *service) validateVolumeCapabilities(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (interface{}, error) {

	switch treq := req.(type) {
	case hasVolumeCapability:
		err := isVolumeCapabilitySupported(treq.GetVolumeCapability())
		if err != nil {
			return nil, err
		}
	case hasVolumeCapabilities:
		err := isVolumeCapabilitySupported(treq.GetVolumeCapabilities()...)
		if err != nil {
			return nil, err
		}
	}

	return handler(ctx, req)
}

// isVolumeCapabilitySupported returns a flag indicating whether not the
// supplied one or several volume capabilities are allowed by this SP.
func isVolumeCapabilitySupported(a ...*csi.VolumeCapability) error {
	if len(a) == 0 {
		return status.Error(
			codes.InvalidArgument, "required: VolumeCapabilities")
	}
	for _, cap := range a {
		if cap.GetBlock() != nil {
			return status.Error(
				codes.InvalidArgument, "unsupported access type: block")
		}
		if am := cap.AccessMode; am != nil {
			switch am.Mode {
			case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY:
			default:
				return status.Errorf(
					codes.InvalidArgument, "unsupported access mode: %v",
					am.Mode)
			}
		}
	}
	return nil
}
