package socks

import (
	"context"
	"io"
	"net"

	"paqet/internal/client"
	"paqet/internal/conf"
	"paqet/internal/flog"
)

type Server struct {
	client   *client.Client
	username string
	password string

	tcp *net.TCPListener
	udp *net.UDPConn
}

func New(client *client.Client) (*Server, error) {
	return &Server{client: client}, nil
}

func (s *Server) Start(ctx context.Context, cfg conf.SOCKS5) error {
	addr, err := net.ResolveTCPAddr("tcp", cfg.Listen.String())
	if err != nil {
		return err
	}
	if s.tcp, err = net.ListenTCP("tcp", addr); err != nil {
		return err
	}
	if s.udp, err = net.ListenUDP("udp", &net.UDPAddr{IP: addr.IP, Port: addr.Port}); err != nil {
		s.tcp.Close()
		return err
	}
	s.username, s.password = cfg.Username, cfg.Password

	flog.Infof("SOCKS5 server listening on %s", addr)
	go s.serveUDP(ctx)
	go s.serve(ctx)

	go func() {
		<-ctx.Done()
		s.tcp.Close()
		s.udp.Close()
		flog.Debugf("SOCKS5 server on %s shut down", addr)
	}()
	return nil
}

func (s *Server) needAuth() bool { return s.username != "" || s.password != "" }

func (s *Server) serve(ctx context.Context) {
	for {
		conn, err := s.tcp.AcceptTCP()
		if err != nil {
			return
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn *net.TCPConn) {
	defer conn.Close()
	if err := s.negotiate(conn); err != nil {
		flog.Debugf("SOCKS5 negotiation with %s failed: %v", conn.RemoteAddr(), err)
		return
	}
	req, err := s.readRequest(conn)
	if err != nil {
		flog.Debugf("SOCKS5 request from %s failed: %v", conn.RemoteAddr(), err)
		return
	}

	switch req.cmd {
	case cmdConnect:
		flog.Debugf("SOCKS5 CONNECT from %s to %s", conn.RemoteAddr(), req.address())
		s.handleConnect(ctx, conn, req)
	case cmdUDP:
		flog.Debugf("SOCKS5 UDP_ASSOCIATE from %s", conn.RemoteAddr())
		s.handleAssociate(ctx, conn)
	default:
		flog.Debugf("SOCKS5 unsupported command %d from %s", req.cmd, conn.RemoteAddr())
		s.writeReply(conn, repCmdUnsupp)
	}
}

func (s *Server) negotiate(conn *net.TCPConn) error {
	var hdr [2]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return err
	}
	if hdr[0] != ver {
		return errProtocol
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}

	want := byte(methodNone)
	if s.needAuth() {
		want = methodAuth
	}
	for _, m := range methods {
		if m == want {
			if _, err := conn.Write([]byte{ver, want}); err != nil {
				return err
			}
			if want == methodAuth {
				return s.checkAuth(conn)
			}
			return nil
		}
	}
	conn.Write([]byte{ver, methodNA})
	return errProtocol
}

func (s *Server) checkAuth(conn *net.TCPConn) error {
	var b [2]byte
	if _, err := io.ReadFull(conn, b[:]); err != nil { // VER + ULEN
		return err
	}
	user := make([]byte, b[1])
	if _, err := io.ReadFull(conn, user); err != nil {
		return err
	}
	if _, err := io.ReadFull(conn, b[1:]); err != nil { // PLEN
		return err
	}
	pass := make([]byte, b[1])
	if _, err := io.ReadFull(conn, pass); err != nil {
		return err
	}
	if string(user) == s.username && string(pass) == s.password {
		_, err := conn.Write([]byte{verAuth, repSuccess})
		return err
	}
	conn.Write([]byte{verAuth, repFailure})
	return errProtocol
}

func (s *Server) readRequest(conn *net.TCPConn) (*request, error) {
	var hdr [3]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return nil, err
	}
	if hdr[0] != ver {
		return nil, errProtocol
	}
	atyp, addr, port, err := readAddr(conn)
	if err != nil {
		return nil, err
	}
	return &request{cmd: hdr[1], atyp: atyp, addr: addr, port: port}, nil
}

func (s *Server) writeReply(conn *net.TCPConn, rep byte) error {
	_, err := conn.Write([]byte{ver, rep, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
	return err
}
