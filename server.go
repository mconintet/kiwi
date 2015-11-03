package kiwi

import (
	"net"
	"sync"
)

const (
	defaultMaxHandshakeBytes = 1 << 20
)

type ConnPool struct {
	p     map[uint64]*Conn
	idx   uint64
	mu    sync.Mutex
	count uint64
}

func NewConnPool() *ConnPool {
	cp := &ConnPool{}
	cp.p = make(map[uint64]*Conn)
	return cp
}

func (cp *ConnPool) Add(c *Conn) {
	cp.mu.Lock()
	cp.idx++
	c.ID = cp.idx
	cp.p[cp.idx] = c
	cp.count++
	cp.mu.Unlock()
}

func (cp *ConnPool) Get(id uint64) (*Conn, bool) {
	cp.mu.Lock()
	c, ok := cp.p[id]
	cp.mu.Unlock()
	return c, ok
}

func (cp *ConnPool) Del(c *Conn) {
	cp.mu.Lock()
	delete(cp.p, c.ID)
	cp.count--
	cp.mu.Unlock()
}

func (cp *ConnPool) Count() uint64 {
	return cp.count
}

type Server struct {
	Addr              *net.TCPAddr
	MaxHandshakeBytes int
	ConnPool          *ConnPool

	handshakeReqRouter OnHandshakeRequestRouter
	onConnOpenRouter   OnConnOpenRouter
	onConnCloseRouter  OnConnCloseRouter
}

func NewServer() *Server {
	srv := &Server{}
	srv.ConnPool = NewConnPool()
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
			srv.ConnPool.Add(conn)
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

	if srv.onConnCloseRouter == nil {
		srv.onConnCloseRouter = DefaultOnConnCloseRouter{}
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

func (srv *Server) OnConnCloseFunc(pattern string, fn OnConnCloseFunc) {
	if srv.onConnCloseRouter.HasHandler(pattern) {
		panic("OnConnCloseFunc already exist with pattern: " + pattern)
	}

	srv.onConnCloseRouter.HandleFunc(pattern, fn)

	if pattern[len(pattern)-1] != '/' {
		pattern += "/"
		srv.onConnCloseRouter.HandleFunc(pattern, fn)
	}
}

func (srv *Server) ListenAndServe() error {
	if ln, err := net.ListenTCP("tcp", srv.Addr); err != nil {
		return err
	} else {
		return srv.serve(ln)
	}
}
