package provider

import (
	"fmt"

	"github.com/rexray/csi-vfs/service"
	"github.com/rexray/gocsi"
)

// New returns a new CSI-VFS Storage Plug-in Provider.
func New() gocsi.StoragePluginProvider {
	svc := service.New()
	return &gocsi.StoragePlugin{
		Controller:  svc,
		Identity:    svc,
		Node:        svc,
		BeforeServe: svc.BeforeServe,
		EnvVars: []string{
			// Enable serial volume access.
			gocsi.EnvVarSerialVolAccess + "=true",

			// Treat the following fields as required:
			//    * ControllerPublishVolumeRequest.NodeId
			//    * GetNodeIDResponse.NodeId
			gocsi.EnvVarRequireNodeID + "=true",

			// Treat the following fields as required:
			//    * ControllerPublishVolumeResponse.PublishVolumeInfo
			//    * NodePublishVolumeRequest.PublishVolumeInfo
			gocsi.EnvVarRequirePubVolInfo + "=true",

			// Provide the list of versions supported by this SP. The
			// specified versions will be:
			//     * Returned by GetSupportedVersions
			//     * Used to validate the Version field of incoming RPCs
			gocsi.EnvVarSupportedVersions + "=" + service.SupportedVersions,

			// Provide the SP's identity information. This will enable the
			// middleware that overrides the GetPluginInfo RPC and returns
			// the following information instead.
			fmt.Sprintf("%s=%s,%s",
				gocsi.EnvVarPluginInfo, service.Name, service.VendorVersion),
		},
	}
}
