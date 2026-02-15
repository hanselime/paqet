package forward

import (
	"context"
	"net"
	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
	"paqet/internal/tnet"
	"time"
)

func (f *Forward) listenUDP(ctx context.Context) {
	laddr, err := net.ResolveUDPAddr("udp", f.listenAddr)
	if err != nil {
		flog.Errorf("failed to resolve UDP listen address '%s': %v", f.listenAddr, err)
		return
	}

	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		flog.Errorf("failed to bind UDP socket on %s: %v", laddr, err)
		return
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	flog.Infof("UDP forwarder listening on %s -> %s", laddr, f.targetAddr)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := f.handleUDPPacket(ctx, conn); err != nil {
			flog.Errorf("UDP packet handling failed on %s: %v", f.listenAddr, err)
		}
	}
}

func (f *Forward) handleUDPPacket(ctx context.Context, conn *net.UDPConn) error {
	buf := make([]byte, buffer.UPool)

	n, caddr, err := conn.ReadFromUDP(buf)
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}

	strm, new, k, err := f.client.UDP(caddr.String(), f.targetAddr)
	if err != nil {
		flog.Errorf("failed to establish UDP stream for %s -> %s: %v", caddr, f.targetAddr, err)
		f.client.CloseUDP(k)
		return err
	}

	if _, err := strm.Write(buf[:n]); err != nil {
		flog.Errorf("failed to forward %d bytes from %s -> %s: %v", n, caddr, f.targetAddr, err)
		f.client.CloseUDP(k)
		return err
	}
	if new {
		flog.Infof("accepted UDP connection %d for %s -> %s", strm.SID(), caddr, f.targetAddr)
		go f.handleUDPStrm(ctx, k, strm, conn, caddr)
	}

	return nil
}

func (f *Forward) handleUDPStrm(ctx context.Context, k uint64, strm tnet.Strm, conn *net.UDPConn, caddr *net.UDPAddr) {
	buf := make([]byte, buffer.UPool)
	defer func() {
		flog.Debugf("UDP stream %d closed for %s -> %s", strm.SID(), caddr, f.targetAddr)
		f.client.CloseUDP(k)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		strm.SetDeadline(time.Now().Add(8 * time.Second))
		n, err := strm.Read(buf)
		_, err = conn.WriteToUDP(buf[:n], caddr)
		strm.SetDeadline(time.Time{})
		if err != nil {
			flog.Errorf("UDP stream %d failed for %s -> %s: %v", strm.SID(), caddr, f.targetAddr, err)
			return
		}
	}
}
