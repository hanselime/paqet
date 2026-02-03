package run

import (
	"log"
	"paqet/internal/conf"
	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"

	"github.com/spf13/cobra"
)

var confPath string

func init() {
	Cmd.Flags().StringVarP(&confPath, "config", "c", "config.yaml", "Path to the configuration file.")
}

var Cmd = &cobra.Command{
	Use:   "run",
	Short: "Runs the client or server based on the config file.",
	Long:  `The 'run' command reads the specified YAML configuration file.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := conf.LoadFromFile(confPath)
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}
		initialize(cfg)

		switch cfg.Role {
		case "client":
			startClient(cfg)
			return
		case "server":
			startServer(cfg)
			return
		}

		log.Fatalf("Failed to load configuration")
	},
}

func initialize(cfg *conf.Conf) {
	flog.SetLevel(cfg.Log.Level)
	buffer.Initialize(cfg.Transport.TCPBuf, cfg.Transport.UDPBuf)
}
