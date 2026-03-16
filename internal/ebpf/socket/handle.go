package socket

import (
	"fmt"
	"net"
	"paqet/internal/conf"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

var (
	bpfObjs tcp2udpObjects
	bpfLink link.Link
	bpfOnce sync.Once
)

func InitBPFHandle(cfg *conf.Network) error {
	var e error

	bpfOnce.Do(func() {
		ifaceName := cfg.Interface.Name
		iface, err := net.InterfaceByName(ifaceName)
		if err != nil {
			e = fmt.Errorf("lookup network iface %q: %w", ifaceName, err)
			return
		}

		if err = rlimit.RemoveMemlock(); err != nil {
			e = fmt.Errorf("remove memlock: %w", err)
			return
		}

		if err := loadTcp2udpObjects(&bpfObjs, nil); err != nil {
			e = fmt.Errorf("loading objects: %w", err)
			return
		}

		bpfLink, err = link.AttachTCX(link.TCXOptions{
			Interface: iface.Index,
			Program:   bpfObjs.TcTcpToPaqet,
			Attach:    ebpf.AttachTCXIngress,
		})
		if err != nil {
			e = fmt.Errorf("could not attach TC program: %s", err)
		}
	})
	return e
}

func updateTargetPorts(port uint32, status uint16) error {
	return bpfObjs.TargetPort.Update(&port, &status, ebpf.UpdateAny)
}
