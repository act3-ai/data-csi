package data

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/mount"
	"oras.land/oras-go/v2/registry/remote/auth"
	credstore "oras.land/oras-go/v2/registry/remote/credentials"

	telemv1alpha2 "github.com/act3-ai/data-telemetry/v3/pkg/apis/config.telemetry.act3-ace.io/v1alpha2"
	"github.com/act3-ai/data-tool/pkg/conf"
	telem "github.com/act3-ai/data-tool/pkg/telemetry"
	tbottle "github.com/act3-ai/data-tool/pkg/transfer/bottle"
	"github.com/act3-ai/go-common/pkg/logger"
)

// NodeServer implements csi.NodeServer.
type NodeServer struct {
	name                string
	nodeID              string
	ephemeralStagingDir string
	cacheDir            string
	telemHosts          []telemv1alpha2.Location
	telemUserName       string

	mounter mount.Interface

	// cacheSizeGauge    prometheus.Gauge
	// bottleSizeGauge   prometheus.Gauge
	// downloadSizeGauge prometheus.Gauge
	bottleCounter prometheus.Counter

	recorder record.EventRecorder
	log      *slog.Logger

	csi.UnimplementedNodeServer // forward compatibility
}

// NewNodeServer creates a new node server with a nodeID.
func NewNodeServer(name, nodeID, storageDir string, telemHosts []telemv1alpha2.Location,
	telemUserName string, log *slog.Logger,
) *NodeServer {
	if name == "" {
		panic("Driver is missing name")
	}

	if nodeID == "" {
		panic("Driver is missing nodeID")
	}

	log.Info("Creating node server", "name", name, "nodeId", nodeID)

	return &NodeServer{
		name:                name,
		nodeID:              nodeID,
		ephemeralStagingDir: filepath.Join(storageDir, "ephemeral", "staging"),
		cacheDir:            filepath.Join(storageDir, "cache"),
		telemHosts:          telemHosts,
		telemUserName:       telemUserName,
		mounter:             mount.New("/busybox/mount"), // We can make this configurable if need be

		// TODO should we be using promauto or explicitly pass in a prometheus Registry?
		// This makes use of a global and thus this object may not be able to be instantiated twice (and testability might be worse).
		// cacheSizeGauge: promauto.NewGauge(prometheus.GaugeOpts{
		// 	Name: "bottle_cache_size",
		// 	Help: "The total size of the cache in bytes",
		// }),
		// bottleSizeGauge: promauto.NewGauge(prometheus.GaugeOpts{
		// 	Name: "bottle_size",
		// 	Help: "The total size of the bottles in bytes",
		// }),
		// downloadSizeGauge: promauto.NewGauge(prometheus.GaugeOpts{
		// 	Name: "bottle_download_size",
		// 	Help: "The amount of data pulled over the network in bytes",
		// }),
		bottleCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "bottle_success_total",
			Help: "The number of bottles successfully created",
		}),
		recorder: createEventRecorder(log, nodeID),
		log:      log,
	}
}

// NodeGetCapabilities returns the supported capabilities of the node server.
func (ns *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	newCap := func(cap csi.NodeServiceCapability_RPC_Type) *csi.NodeServiceCapability {
		return &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: cap,
				},
			},
		}
	}

	caps := []*csi.NodeServiceCapability{
		newCap(csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME),
	}

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

// NodeGetInfo returns basic information about the node.
func (ns *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: ns.nodeID,
		// MaxVolumesPerNode: 10,
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				ns.name: ns.nodeID,
			},
		},
	}, nil
}

// configFile ~/.docker/config.json file info.
type configFile struct {
	AuthConfigs map[string]authConfig `json:"auths"`
}

// authConfig contains authorization information for connecting to a Registry.
type authConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Auth     string `json:"auth,omitempty"`
}

/*
Persistent volumes flow:
1. NodeStageVolume
2. NodePublishVolume
...
3. NodeUnpublishVolume
4. NodeUnstageVolume

See https://kubernetes.io/blog/2020/01/21/csi-ephemeral-inline-volumes/
Ephemeral volumes flow:
1. NodePublishVolume
...
2. NodeUnpublishVolume
There is no stage and unstage.  Also the staging target path is not set.
*/

func (ns *NodeServer) stageVolume(ctx context.Context, stagingPath string, volumeContext map[string]string, credentials map[string]string) error {
	btlRef := volumeContext["bottle"]

	// Extract the selector
	// TODO Consider switching to the form
	// selector_0: foo=bar
	// selector_1: x!=y,z=45
	// This would avoid the need for a delimiter of "|" that might bite us in the future
	// We could also support selecting by part name: e.g., partName_0: my/part
	// We could also support selecting by artifact path: e.g., artifactPath_0: my/path/file.ipynb
	// i.e. this implementation only supports label selection
	selectorString, exists := volumeContext["selector"]
	var labelSelectors []string
	if exists {
		labelSelectors = strings.Split(selectorString, "|") // splits on '|'
	}

	// This logger is a reference to ace-dt's logger
	log := ns.log.With("bottle", btlRef, "selectors", labelSelectors, "stagingPath", stagingPath)
	log.InfoContext(ctx, "Pulling bottle")

	log.DebugContext(ctx, "Parameters", "volumeContext", volumeContext)

	store := credstore.NewMemoryStore()

	// load credentials
	dockerConfig := credentials[".dockerconfigjson"]
	if dockerConfig != "" { // only load credentials if we have them
		dockerCfg := &configFile{}
		if err := json.Unmarshal([]byte(dockerConfig), dockerCfg); err != nil {
			return fmt.Errorf("decoding docker config: %w", err)
		}

		// add credentials to transfer configuration
		for hostname, authCfg := range dockerCfg.AuthConfigs {
			cred := auth.Credential{}
			cred.Username = authCfg.Username
			cred.Password = authCfg.Password
			// TODO which takes precedence in docker (username/password or auth)?
			if authCfg.Auth != "" {
				// base64 decode
				data, err := base64.StdEncoding.DecodeString(authCfg.Auth)
				if err != nil {
					return fmt.Errorf("decoding auth: %w", err)
				}

				// split on : to extract username and password
				parts := strings.Split(string(data), ":")
				if len(parts) != 2 {
					return fmt.Errorf("incorrectly formatted auth")
				}
				cred.Username = parts[0]
				cred.Password = parts[1]
			}
			if err := store.Put(ctx, hostname, cred); err != nil {
				return fmt.Errorf("storing credentials in memory: %w", err)
			}
		}
	}

	// prepare target resolver
	config := conf.New(conf.WithUserAgent("ace-data-csi-driver"), conf.WithCredentialStore(store))
	config.AddConfigOverride(
		conf.WithTelemetry(ns.telemHosts, ns.telemUserName),
		conf.WithCachePath(ns.cacheDir),
	)

	// prepare pull options
	pullOpts := tbottle.PullOptions{
		TransferOptions: tbottle.TransferOptions{
			CachePath: ns.cacheDir,
		},
		PartSelectorOptions: tbottle.PartSelectorOptions{
			Labels: labelSelectors,
		},
	}

	podRef := getPodReference(volumeContext)
	if ns.recorder != nil {
		ns.recorder.Eventf(podRef, corev1.EventTypeNormal, BottlePulling, "Pulling bottle %s", btlRef)
	}
	startTime := time.Now()
	pullCtx := logger.NewContext(ctx, log.WithGroup("tool")) // used for ace-dt API

	// resolve with telemetry
	log.InfoContext(ctx, "resolivng bottle reference with telemetry", "btlRef", btlRef)
	telemAdapt := telem.NewAdapter(ctx, ns.telemHosts, ns.telemUserName)
	src, desc, event, err := telemAdapt.ResolveWithTelemetry(pullCtx, btlRef, config, pullOpts.TransferOptions)
	if err != nil {
		return fmt.Errorf("resolving bottle reference with telemetry: %w", err)
	}

	// pull bottle
	log.InfoContext(ctx, "pulling bottle", "pullDir", stagingPath)
	err = tbottle.Pull(pullCtx, src, desc, stagingPath, pullOpts)
	if err != nil {
		if ns.recorder != nil {
			ns.recorder.Eventf(podRef, corev1.EventTypeWarning, BottleFailed, "Failed to pull bottle %s: %s", btlRef, err.Error())
		}
		return fmt.Errorf("pulling bottle: %w", status.Error(codes.Internal, err.Error()))
	}

	log.InfoContext(ctx, "Bottle pull successful")
	if ns.recorder != nil {
		ns.recorder.Eventf(podRef, corev1.EventTypeNormal, BottlePulled, "Successfully pulled bottle %s in %s seconds", btlRef, time.Since(startTime))
	}

	// notify telemetry
	log.InfoContext(ctx, "notifying telemetry")
	_, err = telemAdapt.NotifyTelemetry(pullCtx, src, desc, stagingPath, event)
	if err != nil && !errors.Is(err, telem.ErrTelemetrySend) { // ignore telemetry send failure
		return fmt.Errorf("notifying telemetry: %w", err)
	}

	ns.bottleCounter.Inc()
	return nil
}

// NodeStageVolume fetches the data from OCI and stages it in a directory.
func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	ctx = logger.NewContext(ctx, ns.log) // add logger to ctx
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID must be provided")
	}

	stagingPath := req.GetStagingTargetPath()
	if stagingPath == "" {
		return nil, status.Error(codes.InvalidArgument, "Staging Target Path must be provided")
	}

	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capability must be provided")
	}

	if err := ns.stageVolume(ctx, stagingPath, req.GetVolumeContext(), req.GetSecrets()); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

// NodePublishVolume makes the data available (read-only) to the pod by bind mounting in the target location.
func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	ctx = logger.NewContext(ctx, ns.log) // add logger to ctx

	// Check arguments
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	stagingPath := req.GetStagingTargetPath()

	log := ns.log.With("stagingPath", stagingPath, "targetPath", targetPath, "volumeID", volumeID) //nolint:sloglint

	if req.GetVolumeContext()["csi.storage.k8s.io/ephemeral"] == "true" {
		// we need to make our own stagingPath
		stagingPath = filepath.Join(ns.ephemeralStagingDir, volumeID)
		log.InfoContext(ctx, "Staging ephemeral volume")

		if err := ns.stageVolume(ctx, stagingPath, req.GetVolumeContext(), req.GetSecrets()); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if stagingPath == "" {
		return nil, status.Error(codes.InvalidArgument, "Staging Target Path must be provided")
	}

	notMnt, err := mount.IsNotMountPoint(ns.mounter, targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(targetPath, 0o750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		// to be idempotent
		log.DebugContext(ctx, "Already mounted")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	readOnly := req.GetReadonly()
	attrib := req.GetVolumeContext()
	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()

	log.InfoContext(ctx, "Mounting", "fsType", fsType, "readOnly", readOnly, "attributes", attrib, "mountFlags", mountFlags)

	options := []string{"bind", "noexec"}
	if readOnly {
		options = append(options, "ro")
	}

	if err := ns.mounter.Mount(stagingPath, targetPath, "", options); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the volume making it no longer available to the pod.
func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	ctx = logger.NewContext(ctx, ns.log) // add logger to ctx

	// Check arguments
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	log := ns.log.With("targetPath", targetPath, "volumeID", volumeID) //nolint:sloglint

	// Unmount only if the target path is really a mount point.
	if notMnt, err := mount.IsNotMountPoint(ns.mounter, targetPath); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if !notMnt {
		// Unmounting the image or filesystem.
		log.InfoContext(ctx, "Unmounting...")
		err = ns.mounter.Unmount(targetPath)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// Delete the mount point.
	if err := os.RemoveAll(targetPath); err != nil {
		// Does not return error for non-existent path, repeated calls OK for idempotency.
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	log.InfoContext(ctx, "Volume is unmounted.")

	// cleanup the staging target path for ephemeral volumes
	// NodeUnstageVolume is not called for ephemeral volumes
	// we need to make our own stagingPath
	stagingPath := filepath.Join(ns.ephemeralStagingDir, volumeID)
	if _, err := os.Stat(stagingPath); err == nil {
		// ephemeral stagingPath exists, we need to remove it to cleanup
		log.InfoContext(ctx, "Unstaging ephemeral volume", "stagingPath", stagingPath)
		err := os.RemoveAll(stagingPath)
		if err != nil {
			// handle errors if it is already removed. (idempotency)
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeUnstageVolume removed the staging directory (to free up space).
func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	// ctx = logger.NewContext(ctx, ns.log) // add logger to ctx

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnstageVolume Volume ID must be provided")
	}

	stagingPath := req.GetStagingTargetPath()
	if stagingPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnstageVolume Staging Target Path must be provided")
	}

	err := os.RemoveAll(stagingPath)
	if err != nil {
		// handle errors if it is already removed.
		if !os.IsNotExist(err) {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}
