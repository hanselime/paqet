package ping

import (
	"context"
	"log"

	"paqet/internal/conf"
	"paqet/internal/socket"

	"github.com/spf13/cobra"
)

var (
	confPath string
	payload  string
)

func init() {
	Cmd.Flags().StringVarP(&confPath, "config", "c", "config.yaml", "Path to the configuration file.")
	Cmd.Flags().StringVar(&payload, "payload", "PING", "The string payload to send in the packet")
}

var Cmd = &cobra.Command{
	Use:   "ping [flags]",
	Short: "Sends a single raw TCP packet with a custom payload.",
	Run: func(cmd *cobra.Command, args []string) {
		sendPacket()
	},
}

func sendPacket() {
	cfg, err := conf.LoadFromFile(confPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if cfg.Role != "client" {
		log.Fatalf("ping command requires client configuration")
	}

	netCfg := cfg.Network
	packetConn, err := socket.New(context.TODO(), &netCfg)
	if err != nil {
		log.Fatalf("Failed to create raw socket: %v", err)
	}
	defer packetConn.Close()

	log.Printf("Sending packet from IPv4:%s IPv6:%s to %s via %s...", cfg.Network.IPv4.Addr, cfg.Network.IPv6.Addr, cfg.Server.Addr.String(), cfg.Network.Interface.Name)
	log.Printf("Payload: \"%s\" (%d bytes)", payload, len(payload))

	if _, err := packetConn.WriteTo([]byte(payload), cfg.Server.Addr); err != nil {
		log.Fatalf("Failed to send packet: %v", err)
	}
	log.Printf("Packet sent successfully!")
}
