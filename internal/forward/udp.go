package forward

import (
	"context"
	"net"
	"time"

	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
	"paqet/internal/tnet"
)

func (f *Forward) serveUDP(ctx context.Context, conn *net.UDPConn) {
	flog.Infof("UDP forwarder listening on %s -> %s", f.listenAddr, f.targetAddr)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		f.handleUDPPacket(ctx, conn)
	}
}

func (f *Forward) handleUDPPacket(ctx context.Context, conn *net.UDPConn) {
	buf := make([]byte, buffer.UPool)

	n, caddr, err := conn.ReadFromUDP(buf)
	if err != nil {
		return
	}
	if n == 0 {
		return
	}

	strm, new, k, err := f.client.UDP(caddr.String(), f.targetAddr)
	if err != nil {
		flog.Errorf("failed to establish UDP stream for %s -> %s: %v", caddr, f.targetAddr, err)
		f.client.CloseUDP(k)
		return
	}

	if _, err := strm.Write(buf[:n]); err != nil {
		flog.Errorf("failed to forward %d bytes from %s -> %s: %v", n, caddr, f.targetAddr, err)
		f.client.CloseUDP(k)
		return
	}
	if new {
		flog.Infof("accepted UDP connection %d for %s -> %s", strm.SID(), caddr, f.targetAddr)
		go func() {
			defer f.client.CloseUDP(k)
			f.handleUDPStrm(ctx, strm, conn, caddr)
		}()
	}
}

func (f *Forward) handleUDPStrm(ctx context.Context, strm tnet.Strm, conn *net.UDPConn, caddr *net.UDPAddr) {
	buf := make([]byte, buffer.UPool)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		strm.SetDeadline(time.Now().Add(8 * time.Second))
		n, err := strm.Read(buf)
		strm.SetDeadline(time.Time{})
		if err != nil {
			flog.Errorf("UDP stream %d read failed for %s -> %s: %v", strm.SID(), caddr, f.targetAddr, err)
			return
		}
		_, err = conn.WriteToUDP(buf[:n], caddr)
		if err != nil {
			flog.Errorf("UDP stream %d write failed for %s -> %s: %v", strm.SID(), caddr, f.targetAddr, err)
			return
		}
	}
}
