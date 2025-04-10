package cli

import (
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/act3-ai/data-csi/pkg/data"
	telemv1alpha2 "github.com/act3-ai/data-telemetry/v3/pkg/apis/config.telemetry.act3-ace.io/v1alpha2"
	"github.com/act3-ai/go-common/pkg/config"
	"github.com/act3-ai/go-common/pkg/logger"
	"github.com/act3-ai/go-common/pkg/redact"
	"github.com/act3-ai/go-common/pkg/version"
)

// NewServeCmd creates a new "serve" subcommand.
func NewServeCmd(info version.Info) *cobra.Command {
	var endpoint string
	var name string
	var nodeID string
	var storageDir string
	var pruneSize string
	var telemetryURL string
	var prunePeriod time.Duration

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the CSI driver server",
		Long:  `Listens on a UNIX socket or TCP socket for GRPC method calls from Kubelet.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			log := logger.FromContext(cmd.Context())

			pruneSize, err := humanize.ParseBytes(pruneSize)
			if err != nil {
				return fmt.Errorf("error parsing pruneSize: %w", err)
			}
			log.Info("Prune config", "size", pruneSize, "period", prunePeriod)

			var telemHosts []telemv1alpha2.Location
			var telemUserName string
			// TODO support multiple telemetry servers and all authentication options
			if telemetryURL != "" {
				log.Info("Using telemetry", "url", telemetryURL)
				telemHosts = append(telemHosts, telemv1alpha2.Location{URL: redact.SecretURL(telemetryURL)})
				telemUserName = "csi-driver"
			} else {
				log.Info("WARNING: Not using a telemetry server.  Consider configuring one with --telemetry.")
			}

			driver := data.NewDriver(
				name,
				nodeID,
				info.Version,
				endpoint,
				storageDir,
				pruneSize,
				prunePeriod,
				telemHosts,
				telemUserName,
				log.WithGroup("csi"),
			)

			return driver.Run(cmd.Context())
		},
	}

	cmd.Flags().StringVar(&endpoint, "endpoint", config.EnvOr("ACE_DATA_CSI_ENDPOINT", "unix:///tmp/csi/csi.sock"), "CSI endpoint to listen on")

	cmd.Flags().StringVar(&name, "name", config.EnvOr("ACE_DATA_CSI_NAME", "bottle.csi.act3-ace.io"), "name of the driver")

	cmd.Flags().StringVar(&nodeID, "nodeid", config.EnvOr("ACE_DATA_CSI_NODEID", "nodeid"), "node id")

	cmd.Flags().StringVar(&storageDir, "storagedir", config.EnvOr("ACE_DATA_CSI_STORAGEDIR", "/tmp/csi/data"), "Root path for the node local data storage")

	cmd.Flags().StringVar(&telemetryURL, "telemetry", config.EnvOr("ACE_DATA_CSI_TELEMETRY", ""), "URL of the Telemetry Server")

	cmd.Flags().StringVar(&pruneSize, "prunesize", config.EnvOr("ACE_DATA_CSI_PRUNESIZE", "10Gi"), "Max size of cache in bytes.  SI suffixes are allowed.  Examples: 1 Gi, 50 G, 1 Ti")

	cmd.Flags().DurationVar(&prunePeriod, "pruneperiod", config.EnvDurationOr("ACE_DATA_CSI_PRUNEPERIOD", 24*time.Hour), "Time between pruning runs.  Examples: 1m, 1h, 12h, 72h")

	return cmd
}
