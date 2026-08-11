package forward

import (
	"context"
	"net"
	"net/netip"
	"sync"

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

	buf := make([]byte, buffer.UDPSize+1)
	for {
		n, cAddr, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}

		if n > buffer.UDPSize {
			flog.Debugf("UDP %s: datagram too large, at least %d bytes (limit %d)", cAddr, n, buffer.UDPSize)
			continue
		}
		s := f.openUDPSess(ctx, conn, cAddr)
		if len(s.ch) == cap(s.ch) {
			continue
		}
		d := make([]byte, n)
		copy(d, buf[:n])
		select {
		case s.ch <- d:
		default:
		}
	}
}

func (f *Forward) openUDPSess(ctx context.Context, conn *net.UDPConn, cAddr netip.AddrPort) *udpSess {
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
	sess := &udpSess{ch: make(chan []byte, 64), done: make(chan struct{})}
	f.udpPool[cAddr] = sess
	go f.udpToStrm(ctx, conn, cAddr, sess)
	return sess
}

func (f *Forward) udpToStrm(ctx context.Context, conn *net.UDPConn, cAddr netip.AddrPort, sess *udpSess) {
	strm, _, k, err := f.client.UDP(ctx, cAddr.String(), f.targetAddr)
	if err != nil {
		flog.Errorf("failed to establish UDP stream for %s -> %s: %v", cAddr, f.targetAddr, err)
		f.closeUDPSess(cAddr, sess)
		return
	}
	sess.strm = strm
	sess.key = k
	defer f.closeUDPSess(cAddr, sess)

	flog.Infof("accepted UDP connection %d for %s -> %s", strm.SID(), cAddr, f.targetAddr)
	go func() {
		defer f.closeUDPSess(cAddr, sess)
		f.strmToUDP(ctx, strm, conn, cAddr, sess)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sess.done:
			return
		case buf := <-sess.ch:
			_, err = strm.Write(buf)
			if err != nil {
				flog.Errorf("failed to forward bytes from %s -> %s: %v", cAddr, f.targetAddr, err)
				return
			}
			f.client.Touch(sess.key)
		}
	}
}

func (f *Forward) strmToUDP(ctx context.Context, strm tnet.Strm, conn *net.UDPConn, cAddr netip.AddrPort, sess *udpSess) {
	stop := context.AfterFunc(ctx, func() { strm.Close() })
	defer stop()

	buf := make([]byte, buffer.UDPSize)
	for {
		n, err := strm.Read(buf)
		if err != nil {
			flog.Errorf("UDP stream %d read failed for %s -> %s: %v", strm.SID(), cAddr, f.targetAddr, err)
			return
		}
		f.client.Touch(sess.key)
		_, err = conn.WriteToUDPAddrPort(buf[:n], cAddr)
		if err != nil {
			flog.Errorf("UDP stream %d write failed for %s -> %s: %v", strm.SID(), cAddr, f.targetAddr, err)
			return
		}
	}
}

func (f *Forward) closeUDPSess(cAddr netip.AddrPort, sess *udpSess) {
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
