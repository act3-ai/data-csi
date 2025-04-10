package data

import (
	"context"
	"log/slog"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// IdentityServer implements csi.IdentityServer.
type IdentityServer struct {
	name    string
	version string
	log     *slog.Logger

	csi.UnimplementedIdentityServer // forward compatibility
}

// NewIdentityServer create a new identity server with a given name and version.
func NewIdentityServer(name, version string, log *slog.Logger) *IdentityServer {
	if name == "" {
		panic("Driver name not configured")
	}

	if version == "" {
		panic("Driver is missing version")
	}

	log.Info("Creating identity server", "name", name, "version", version)

	return &IdentityServer{
		name:    name,
		version: version,
		log:     log,
	}
}

// GetPluginInfo conforms to CSI spec.
func (ids *IdentityServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{
		Name:          ids.name,
		VendorVersion: ids.version,
	}, nil
}

// Probe conforms to CSI spec.
func (ids *IdentityServer) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	return &csi.ProbeResponse{}, nil
}

// GetPluginCapabilities conforms to CSI spec.
func (ids *IdentityServer) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	newCap := func(capability csi.PluginCapability_Service_Type) *csi.PluginCapability {
		return &csi.PluginCapability{
			Type: &csi.PluginCapability_Service_{
				Service: &csi.PluginCapability_Service{
					Type: capability,
				},
			},
		}
	}

	caps := []*csi.PluginCapability{
		newCap(csi.PluginCapability_Service_CONTROLLER_SERVICE),
		newCap(csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS),
	}

	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}
