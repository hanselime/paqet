package forward

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
	"paqet/internal/tnet"
)

type udpSess struct {
	strm  tnet.Strm
	ch    chan []byte
	done  chan struct{}
	key   uint64
	close sync.Once
}

func (f *Forward) serveUDP(ctx context.Context, conn *net.UDPConn) {
	flog.Infof("UDP forwarder listening on %s -> %s", f.listenAddr, f.targetAddr)

	for {
		buf := make([]byte, buffer.UPool)
		n, cAddr, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}

		s := f.handleUDPSess(ctx, conn, cAddr)
		select {
		case s.ch <- buf[:n]:
		default:
		}
	}
}

func (f *Forward) handleUDPSess(ctx context.Context, conn *net.UDPConn, cAddr netip.AddrPort) *udpSess {
	f.udpMu.RLock()
	if sess, ok := f.udpPool[cAddr]; ok {
		f.udpMu.RUnlock()
		return sess
	}
	f.udpMu.RUnlock()

	f.udpMu.Lock()
	defer f.udpMu.Unlock()
	if sess, ok := f.udpPool[cAddr]; ok {
		return sess
	}
	sess := &udpSess{ch: make(chan []byte, 128), done: make(chan struct{})}
	f.udpPool[cAddr] = sess
	go f.handleUDPConn(ctx, conn, cAddr, sess)
	return sess
}

func (f *Forward) handleUDPConn(ctx context.Context, conn *net.UDPConn, cAddr netip.AddrPort, sess *udpSess) {
	strm, _, k, err := f.client.UDP(ctx, cAddr.String(), f.targetAddr)
	if err != nil {
		flog.Errorf("failed to establish UDP stream for %s -> %s: %v", cAddr, f.targetAddr, err)
		f.closeUDP(cAddr, sess)
		return
	}
	sess.strm = strm
	sess.key = k
	defer f.closeUDP(cAddr, sess)

	flog.Infof("accepted UDP connection %d for %s -> %s", strm.SID(), cAddr, f.targetAddr)
	go func() {
		defer f.closeUDP(cAddr, sess)
		f.handleUDPStrm(ctx, strm, conn, cAddr)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sess.done:
			return
		case buf := <-sess.ch:
			strm.SetWriteDeadline(time.Now().Add(8 * time.Second))
			_, err = strm.Write(buf)
			strm.SetWriteDeadline(time.Time{})
			if err != nil {
				flog.Errorf("failed to forward bytes from %s -> %s: %v", cAddr, f.targetAddr, err)
				return
			}
		}
	}
}

func (f *Forward) handleUDPStrm(ctx context.Context, strm tnet.Strm, conn *net.UDPConn, cAddr netip.AddrPort) {
	stop := context.AfterFunc(ctx, func() { strm.Close() })
	defer stop()

	buf := make([]byte, buffer.UPool)
	for {
		strm.SetReadDeadline(time.Now().Add(8 * time.Second))
		n, err := strm.Read(buf)
		strm.SetReadDeadline(time.Time{})
		if err != nil {
			flog.Errorf("UDP stream %d read failed for %s -> %s: %v", strm.SID(), cAddr, f.targetAddr, err)
			return
		}
		_, err = conn.WriteToUDPAddrPort(buf[:n], cAddr)
		if err != nil {
			flog.Errorf("UDP stream %d write failed for %s -> %s: %v", strm.SID(), cAddr, f.targetAddr, err)
			return
		}
	}
}

func (f *Forward) closeUDP(cAddr netip.AddrPort, sess *udpSess) {
	sess.close.Do(func() {
		close(sess.done)
		if sess.strm != nil {
			f.client.CloseUDP(sess.key, sess.strm)
		}
		f.udpMu.Lock()
		if s, ok := f.udpPool[cAddr]; ok && s == sess {
			delete(f.udpPool, cAddr)
		}
		f.udpMu.Unlock()
	})
}
