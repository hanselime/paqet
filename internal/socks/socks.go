package socks

import (
	"context"
	"io"
	"net"
	"time"

	"paqet/internal/client"
	"paqet/internal/conf"
	"paqet/internal/flog"
)

type Server struct {
	client   *client.Client
	username string
	password string
}

func New(client *client.Client) (*Server, error) {
	return &Server{client: client}, nil
}

func (s *Server) Start(ctx context.Context, cfg conf.SOCKS5) error {
	s.username, s.password = cfg.Username, cfg.Password

	listener, err := net.Listen("tcp", cfg.Listen.String())
	if err != nil {
		return err
	}
	flog.Infof("SOCKS5 server listening on %s", cfg.Listen.String())

	go s.serveTCP(ctx, listener)
	context.AfterFunc(ctx, func() { listener.Close() })

	return nil
}

func (s *Server) serveTCP(ctx context.Context, listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}

		go func() {
			defer conn.Close()
			s.handleTCPConn(ctx, conn)
		}()
	}
}

func (s *Server) handleTCPConn(ctx context.Context, conn net.Conn) {
	conn.SetDeadline(time.Now().Add(8 * time.Second))
	if err := s.negotiate(conn); err != nil {
		flog.Debugf("SOCKS5 negotiation with %s failed: %v", conn.RemoteAddr(), err)
		return
	}
	req, err := s.read(conn)
	if err != nil {
		flog.Debugf("SOCKS5 request from %s failed: %v", conn.RemoteAddr(), err)
		return
	}
	conn.SetDeadline(time.Time{})

	switch req.cmd {
	case cmdConnect:
		flog.Debugf("SOCKS5 CONNECT from %s to %s", conn.RemoteAddr(), req.address())
		s.handleConnect(ctx, conn, req)
	case cmdUDP:
		flog.Debugf("SOCKS5 UDP_ASSOCIATE from %s", conn.RemoteAddr())
		s.handleAssociate(ctx, conn, req)
	default:
		flog.Debugf("SOCKS5 unsupported command %d from %s", req.cmd, conn.RemoteAddr())
		s.write(conn, repCmdUnsupp)
	}
}

func (s *Server) negotiate(conn net.Conn) error {
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
	if s.username != "" || s.password != "" {
		want = methodAuth
	}
	for _, m := range methods {
		if m == want {
			if _, err := conn.Write([]byte{ver, want}); err != nil {
				return err
			}
			if want == methodAuth {
				return s.authenticate(conn)
			}
			return nil
		}
	}
	conn.Write([]byte{ver, methodNA})
	return errProtocol
}

func (s *Server) authenticate(conn net.Conn) error {
	var b [2]byte
	if _, err := io.ReadFull(conn, b[:]); err != nil { // VER + ULEN
		return err
	}
	if b[0] != verAuth {
		return errProtocol
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

func (s *Server) read(conn net.Conn) (*request, error) {
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

func (s *Server) write(conn net.Conn, rep byte) error {
	_, err := conn.Write([]byte{ver, rep, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
	return err
}
