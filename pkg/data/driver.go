// Package data contains the core components of the CSI bottle driver
package data

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	telemv1alpha1 "gitlab.com/act3-ai/asce/data/telemetry/pkg/apis/config.telemetry.act3-ace.io/v1alpha1"
	"gitlab.com/act3-ai/asce/data/tool/pkg/cache"
)

// Driver object s the top level object for this package.
type Driver struct {
	endpoint      string
	pruneSize     uint64
	prunePeriod   time.Duration
	telemHosts    []telemv1alpha1.Location
	telemUserName string
	log           *slog.Logger

	ids *IdentityServer
	cs  *ControllerServer
	ns  *NodeServer

	srv *grpc.Server
}

// NewDriver creates a new CSI driver.
func NewDriver(name, nodeID, version, endpoint, storageDir string,
	pruneSize uint64, prunePeriod time.Duration, telemHosts []telemv1alpha1.Location,
	telemUserName string, log *slog.Logger,
) *Driver {
	log.Info("Creating driver",
		"name", name,
		"nodeId", nodeID,
		"version", version,
		"storageDir", storageDir,
		"pruneSize", pruneSize,
		"prunePeriod", prunePeriod,
	)

	return &Driver{
		endpoint,
		pruneSize,
		prunePeriod,
		telemHosts,
		telemUserName,
		log.WithGroup("driver"),
		NewIdentityServer(name, version, log.WithGroup("identity")),
		NewControllerServer(name, nodeID, log.WithGroup("controller")),
		NewNodeServer(name, nodeID, storageDir, telemHosts, telemUserName, log.WithGroup("node")),
		nil,
	}
}

// log response errors for better observability.
func (d *Driver) errHandler(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	log := d.log.With("method", info.FullMethod)
	log.DebugContext(ctx, "GRPC called", "request", protosanitizer.StripSecrets(req))
	resp, err := handler(ctx, req)
	if err != nil {
		log.ErrorContext(ctx, "GPRC method failed", "error", err, "request", protosanitizer.StripSecrets(req))
	} else {
		log.DebugContext(ctx, "GRPC method succeeded", "response", resp)
	}
	return resp, err
}

// Run starts the CSI plugin by communication over the given CSIAddress.
func (d *Driver) Run(ctx context.Context) error {
	u, err := url.Parse(d.endpoint)
	if err != nil {
		return fmt.Errorf("unable to parse address: %w", err)
	}

	var addr string
	switch u.Scheme {
	case "unix":
		addr = u.Path
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", addr, err)
		}

		listenDir := filepath.Dir(addr)
		if _, err := os.Stat(listenDir); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("expected Kubelet plugin watcher to create parent dir %s but did not find such a dir", listenDir)
			}
			return fmt.Errorf("failed to stat %s: %w", listenDir, err)
		}
	case "tcp":
		addr = u.Host
	default:
		return fmt.Errorf("%v endpoint scheme not supported", u.Scheme)
	}

	d.log.InfoContext(ctx, "Starting to listen", "scheme", u.Scheme, "address", addr)

	listener, err := net.Listen(u.Scheme, addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	d.srv = grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpc_prometheus.UnaryServerInterceptor,
			d.errHandler,
		),
	)

	// register all my services
	csi.RegisterIdentityServer(d.srv, d.ids)
	csi.RegisterControllerServer(d.srv, d.cs)
	csi.RegisterNodeServer(d.srv, d.ns)

	grpc_prometheus.Register(d.srv)
	grpc_prometheus.EnableHandlingTimeHistogram(
		grpc_prometheus.WithHistogramBuckets([]float64{0.1, 1, 10, 100, 1000}),
	)

	// Register Prometheus metrics handler.
	http.Handle("/metrics", promhttp.Handler())

	g, gctx := errgroup.WithContext(ctx)

	ticker := time.NewTicker(d.prunePeriod)
	defer ticker.Stop()

	g.Go(func() error {
		d.log.Info("Starting pruner")
		for {
			select {
			case tm := <-ticker.C:
				log := d.log.With("tick", tm)
				log.Info("Starting to prune the cache")
				// This can take a while
				e := d.prune(gctx)
				if e != nil {
					log.Error(fmt.Errorf("failed to prune cache: %w", err).Error())
					// just continue on as this is not fatal
					continue
				}
				log.Info("Finished pruning the cache")
			case <-gctx.Done():
				return gctx.Err()
			}
		}
	})

	// Start the http server for prometheus.
	g.Go(func() error {
		d.log.Info("HTTP server started")
		return http.ListenAndServe(":9102", nil)
	})

	g.Go(func() error {
		d.log.Info("GRPC server started")
		return d.srv.Serve(listener)
	})

	return g.Wait()
}

// prune will prune the blob cache.
func (d *Driver) prune(ctx context.Context) error {
	if err := cache.Prune(ctx, d.ns.cacheDir, int64(d.pruneSize)); err != nil {
		return fmt.Errorf("pruning cache: %w", err)
	}
	return nil
}
