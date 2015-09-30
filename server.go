package kiwi

import (
	"net"
	"sync/atomic"
)

const (
	defaultMaxHandshakeBytes = 1 << 20
)

type Server struct {
	Addr              *net.TCPAddr
	MaxHandshakeBytes int
	connCount         int64

	handshakeReqRouter OnHandshakeRequestRouter
	onConnOpenRouter   OnConnOpenRouter
}

func NewServer() *Server {
	srv := &Server{}
	return srv
}

func (srv *Server) serve(ln *net.TCPListener) error {
	defer ln.Close()

	for {
		if cn, err := ln.Accept(); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				continue
			} else {
				return err
			}
		} else {
			conn := newConn(srv, cn)
			atomic.AddInt64(&srv.connCount, 1)
			go conn.serve()
		}
	}
}

func (srv *Server) ApplyDefaultCfg() {
	if srv.MaxHandshakeBytes == 0 {
		srv.MaxHandshakeBytes = defaultMaxHandshakeBytes
	}

	if srv.onConnOpenRouter == nil {
		srv.onConnOpenRouter = DefaultOnConnOpenRouter{}
	}
}

func (srv *Server) OnConnOpenFunc(pattern string, fn OnConnOpenFunc) {
	if srv.onConnOpenRouter.HasHandler(pattern) {
		panic("OnConnOpenFunc already exist with pattern: " + pattern)
	}

	srv.onConnOpenRouter.HandleFunc(pattern, fn)

	if pattern[len(pattern)-1] != '/' {
		pattern += "/"
		srv.onConnOpenRouter.HandleFunc(pattern, fn)
	}
}

func (srv *Server) ListenAndServe() error {
	if ln, err := net.ListenTCP("tcp", srv.Addr); err != nil {
		return err
	} else {
		return srv.serve(ln)
	}
}
