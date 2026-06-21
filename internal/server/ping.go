package server

import (
	"paqet/internal/flog"
	"paqet/internal/protocol"
	"paqet/internal/tnet"
)

func (s *Server) handlePing(strm tnet.Strm) {
	flog.Debugf("accepted ping on stream %d from %s", strm.SID(), strm.RemoteAddr())
	p := protocol.Proto{Type: protocol.PPONG}
	if err := p.Write(strm); err != nil {
		flog.Errorf("failed to send pong on stream %d: %v", strm.SID(), err)
	}
	flog.Debugf("sent pong on stream %d", strm.SID())
}
