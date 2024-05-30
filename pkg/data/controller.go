package data

import (
	"context"
	"log/slog"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ControllerServer object complying to the controller service in CSI.
type ControllerServer struct {
	name    string
	nodeID  string
	volumes sync.Map
	log     *slog.Logger
}

// NewControllerServer create a new CSi compliant controller.
func NewControllerServer(name, nodeID string, log *slog.Logger) *ControllerServer {
	if name == "" {
		panic("Driver is missing name")
	}

	if nodeID == "" {
		panic("Driver is missing nodeID")
	}

	log.Info("Creating controller server", "name", name, "nodeId", nodeID)

	return &ControllerServer{
		name:   name,
		nodeID: nodeID,
		log:    log,
	}
}

// ControllerGetCapabilities returns the capabilities of the controller service.
func (cs *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	newCap := func(cap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
		return &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
	}

	caps := []*csi.ControllerServiceCapability{
		newCap(csi.ControllerServiceCapability_RPC_PUBLISH_READONLY),
		// newCap(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME), // Needed for dynamic provisioning
		// newCap(csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME),
	}

	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

// CreateVolume creates a new volume as per the CSI spec.
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {

	// Check arguments
	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "Name missing in request")
	}
	caps := req.GetVolumeCapabilities()
	if caps == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities missing in request")
	}
	for _, cap := range caps {
		if cap.GetBlock() != nil {
			return nil, status.Error(codes.Unimplemented, "Block Volume not supported")
		}
	}

	// if c != csi.ControllerServiceCapability_RPC_UNKNOWN {
	// 	return status.Errorf(codes.InvalidArgument, "unsupported capability %s", c)
	// }

	// for _, cap := range cs.caps {
	// 	if c == cap.GetRpc().GetType() {
	// 		return nil
	// 	}
	// }
	// return status.Errorf(codes.InvalidArgument, "unsupported capability %s", c)

	// Need to check for already existing volume name, and if found
	// check for the requested capacity and already allocated capacity
	// var exVol string
	// cs.volumes.Range(func(volID interface{}, volName interface{}) bool {
	// 	if name == volName {
	// 		exVol = volID
	// 		return true
	// 	}
	// })
	// if exVol != "" {
	// 	// Since err is nil, it means the volume with the same name already exists
	// 	// need to check if the size of exisiting volume is the same as in new
	// 	// request
	// 	if exVol.VolSize >= int64(req.GetCapacityRange().GetRequiredBytes()) {
	// 		// exisiting volume is compatible with new request and should be reused.
	// 		return &csi.CreateVolumeResponse{
	// 			Volume: &csi.Volume{
	// 				VolumeId:      exVol.VolID,
	// 				CapacityBytes: int64(exVol.VolSize),
	// 				VolumeContext: req.GetParameters(),
	// 			},
	// 		}, nil
	// 	}
	// 	return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("Volume with the same name: %s but with different size already exist", req.GetName()))
	// }

	// create a new volume and add it to the collection
	volumeID := uuid.New().String()
	cs.volumes.Store(volumeID, name)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(), // TODO set this to the the data bottle's uncompressed size.
			VolumeContext: req.GetParameters(),
			ContentSource: req.GetVolumeContentSource(),
			AccessibleTopology: []*csi.Topology{
				{ // restricted to the node it is created on
					Segments: map[string]string{
						cs.name: cs.nodeID,
					},
				},
			},
		},
	}, nil
}

// DeleteVolume implements csi.ControllerServer (unimplemented).
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	// Check arguments
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	cs.volumes.Delete(volumeID)

	return &csi.DeleteVolumeResponse{}, nil
}

// ListVolumes implements csi.ControllerServer (unimplemented).
func (cs *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested are supported.
func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume ID must be provided")
	}

	volCaps := req.GetVolumeCapabilities()
	if volCaps == nil {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume Capabilities must be provided")
	}

	// check if the volume exists
	_, ok := cs.volumes.Load(volumeID)
	if !ok {
		return nil, status.Error(codes.NotFound, "ValidateVolumeCapabilities Volume ID does not exist")
	}

	// TODO actually check the capabilities to see if they match

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							// FsType:     "tmpfs",
							// MountFlags: []string{"r"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
					},
				},
			},
			VolumeContext: req.GetVolumeContext(),
			Parameters:    req.GetParameters(),
		},
	}, nil
}

// ControllerExpandVolume implements csi.ControllerServer (unimplemented).
func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerPublishVolume implements csi.ControllerServer (unimplemented).
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerUnpublishVolume implements csi.ControllerServer (unimplemented).
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// GetCapacity implements csi.ControllerServer (unimplemented).
func (cs *ControllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// CreateSnapshot implements csi.ControllerServer (unimplemented).
func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// DeleteSnapshot implements csi.ControllerServer (unimplemented).
func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ListSnapshots implements csi.ControllerServer (unimplemented).
func (cs *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetVolume implements csi.ControllerServer (unimplemented).
func (cs *ControllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerModifyVolume implements csi.ControllerServer (unimplemented).
func (cs *ControllerServer) ControllerModifyVolume(context.Context, *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
