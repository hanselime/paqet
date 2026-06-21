package server

import (
	"context"

	"paqet/internal/flog"
	"paqet/internal/protocol"
	"paqet/internal/tnet"
)

func (s *Server) handleConn(ctx context.Context, conn tnet.Conn) {
	for {
		select {
		case <-ctx.Done():
			flog.Debugf("stopping smux session for %s due to context cancellation", conn.RemoteAddr())
			return
		default:
		}
		strm, err := conn.AcceptStrm()
		if err != nil {
			flog.Errorf("failed to accept stream on %s: %v", conn.RemoteAddr(), err)
			return
		}
		go func() {
			defer strm.Close()
			s.handleStrm(ctx, strm)
			flog.Debugf("stream %d from %s closed", strm.SID(), strm.RemoteAddr())
		}()
	}
}

func (s *Server) handleStrm(ctx context.Context, strm tnet.Strm) {
	var p protocol.Proto
	err := p.Read(strm)
	if err != nil {
		flog.Errorf("failed to read protocol message from stream %d: %v", strm.SID(), err)
		return
	}

	switch p.Type {
	case protocol.PPING:
		s.handlePing(strm)
	case protocol.PTCPF:
		s.pConn.SetClientTCPF(strm.RemoteAddr(), p.TCPF)
	case protocol.PTCP:
		s.handleTCPProtocol(ctx, strm, &p)
	case protocol.PUDP:
		s.handleUDPProtocol(ctx, strm, &p)
	default:
		flog.Errorf("unknown protocol type %d on stream %d", p.Type, strm.SID())
	}
}
