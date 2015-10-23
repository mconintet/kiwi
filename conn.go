package kiwi

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"
)

const (
	StateConnecting int32 = iota
	StateOpen
	StateClosing
	StateClosed
)

type OnHandshakeRequestHandler interface {
	ServeHandshake(*HandshakeRequest, *Conn) (errCode int, err error)
}

type OnHandshakeRequestFunc func(*HandshakeRequest, *Conn) (errCode int, err error)

func (f OnHandshakeRequestFunc) ServeHandshake(h *HandshakeRequest, c *Conn) (errCode int, err error) {
	return f(h, c)
}

type OnHandshakeRequestRouter map[string]OnHandshakeRequestHandler

func (r OnHandshakeRequestRouter) Serve(hsReq *HandshakeRequest, conn *Conn) (errCode int, err error) {
	if handler, ok := r[hsReq.RequestURL.Path]; ok {
		return handler.ServeHandshake(hsReq, conn)
	}
	return DefaultServerHandshakeFunc(hsReq, conn)
}

type OnConnOpenHandler interface {
	ServerConn(MessageReceiver, MessageSender)
}

type OnConnOpenFunc func(r MessageReceiver, s MessageSender)

func (f OnConnOpenFunc) ServerConn(r MessageReceiver, s MessageSender) {
	f(r, s)
}

type OnConnOpenRouter interface {
	HandleFunc(pattern string, fn OnConnOpenFunc)
	HasHandler(reqPath string) bool
	Serve(reqPath string, conn *Conn)
}

type DefaultOnConnOpenRouter map[string]OnConnOpenHandler

func (r DefaultOnConnOpenRouter) HandleFunc(pattern string, fn OnConnOpenFunc) {
	r[pattern] = fn
}

func (r DefaultOnConnOpenRouter) HasHandler(reqPath string) bool {
	_, ok := r[reqPath]
	return ok
}

func (r DefaultOnConnOpenRouter) Serve(reqPath string, conn *Conn) {
	handler := r[reqPath]

	receiver := &DefaultMessageReceiver{}
	receiver.SetConn(conn)

	sender := &DefaultMessageSender{}
	sender.SetConn(conn)

	handler.ServerConn(receiver, sender)
}

type Conn struct {
	ID uint64

	rwc   net.Conn
	state int32

	Server *Server
	Buf    *bufio.ReadWriter

	HandshakeRequest *HandshakeRequest
}

func (c *Conn) Write(p []byte) (n int, err error) {
	if n, err = c.Buf.Write(p); err != nil {
		return n, err
	}
	return n, c.Buf.Flush()
}

func (c *Conn) SetState(state int32) {
	atomic.StoreInt32(&c.state, state)
}

func (c *Conn) GetState() int32 {
	return atomic.LoadInt32(&c.state)
}

func newConn(srv *Server, c net.Conn) *Conn {
	conn := new(Conn)

	conn.Server = srv
	conn.rwc = c

	br := bufio.NewReader(c)
	bw := bufio.NewWriter(c)
	conn.Buf = bufio.NewReadWriter(br, bw)

	conn.SetState(StateConnecting)

	return conn
}

func (c *Conn) doHandshake() (errCode int, err error) {
	hsReq := &HandshakeRequest{}
	if err := hsReq.ReadFrom(c.Buf, c.Server.MaxHandshakeBytes); err != nil {
		return 400, err
	}

	c.HandshakeRequest = hsReq
	return c.Server.handshakeReqRouter.Serve(hsReq, c)
}

func (c *Conn) Close() {
	c.rwc.Close()
	c.Server.ConnPool.Del(c)
}

func (c *Conn) FailHandshake(code int, err error) {
	buf := c.Buf
	fmt.Fprintf(buf, "HTTP/1.1 %03d %s\r\n", code, http.StatusText(code))
	buf.WriteString("\r\n")
	buf.WriteString(err.Error() + "\n")
	buf.Flush()
	c.Close()

	log.Printf("[Handshake] %s\n", err.Error())
}

func (c *Conn) serve() {
	// do handshake
	if errCode, err := c.doHandshake(); err != nil {
		c.FailHandshake(errCode, err)
		return
	}

	c.SetState(StateOpen)

	// data transform
	c.Server.onConnOpenRouter.Serve(c.HandshakeRequest.RequestURL.Path, c)
}

var ErrNotSupportedVersion = &ProtocolError{"not supported version"}

func DefaultServerHandshakeCheck(hsReq *HandshakeRequest, conn *Conn) (errCode int, err error) {
	header := hsReq.Header

	if hsReq.ProtoVer != "1.1" {
		return http.StatusBadRequest, &ProtocolError{"invalid http proto ver"}
	}

	if !header.HasKey("Host") {
		return http.StatusBadRequest, &ProtocolError{"missing header 'Host'"}
	}

	// ff 40.0.3 gives "keep-alive, Upgrade"
	if !header.HasKeyAndValContains("Connection", "Upgrade") {
		return http.StatusBadRequest, &ProtocolError{"missing or invalid header 'Connection'"}
	}

	if !header.HasKeyAndValEqual("Upgrade", "websocket") {
		return http.StatusBadRequest, &ProtocolError{"missing or invalid header 'Upgrade'"}
	}

	if !header.HasKeyAndValEqual("Sec-WebSocket-Version", "13") {
		return http.StatusBadRequest, &ProtocolError{"missing or invalid header 'Sec-WebSocket-Version'"}
	}

	if !header.HasKey("Sec-WebSocket-Version") {
		return http.StatusBadRequest, &ProtocolError{"missing header 'Sec-WebSocket-Version'"}
	} else if header.GetOne("Sec-WebSocket-Version") != "13" {
		return http.StatusBadRequest, ErrNotSupportedVersion
	}

	if !header.HasKey("Sec-WebSocket-Key") {
		return http.StatusBadRequest, &ProtocolError{"missing header 'Sec-WebSocket-Key"}
	}

	if !conn.Server.onConnOpenRouter.HasHandler(hsReq.RequestURL.Path) {
		return http.StatusNotFound, &ProtocolError{"service not found for: " + hsReq.RequestURL.Path}
	}

	return 0, nil
}

func DefaultServerHandshakeFunc(hsReq *HandshakeRequest, conn *Conn) (errCode int, err error) {
	if errCode, err = DefaultServerHandshakeCheck(hsReq, conn); err != nil {
		return
	}

	key := hsReq.Header.GetOne("Sec-WebSocket-Key")
	respKey := MakeAcceptKey(key)

	buf := conn.Buf
	buf.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	buf.WriteString("Upgrade: websocket\r\n")
	buf.WriteString("Connection: Upgrade\r\n")
	buf.WriteString("Sec-WebSocket-Accept: " + string(respKey) + "\r\n")
	buf.WriteString("\r\n")
	buf.Flush()

	return
}
