package kiwi

import (
	"errors"
	"io"
	"sync"
)

type Message struct {
	Opcode uint8
	Data   []byte
}

func (m *Message) IsClose() bool {
	return m.Opcode == OpcodeClose
}

func (m *Message) IsText() bool {
	return m.Opcode == OpcodeText
}

func (m *Message) IsBinary() bool {
	return m.Opcode == OpcodeBinary
}

func (m *Message) IsPing() bool {
	return m.Opcode == OpcodePing
}

func (m *Message) IsPong() bool {
	return m.Opcode == OpcodePong
}

type MessageReceiver interface {
	SetConn(c *Conn) MessageReceiver
	GetConn() *Conn

	ReadWhole(maxMsgDataLen uint64) (msg *Message, err error)

	BeginReadFrame()
	ReadFrame(maxFramePayloadLen uint64) (frame *Frame, fin bool, err error)
	EndReadFrame()

	IsConnOpen() bool
}

var (
	ErrConnIsNotOpen   = errors.New("conn is not open")
	ErrMessageTooLarge = errors.New("message too large")
)

type DefaultMessageReceiver struct {
	conn *Conn
	mu   sync.Mutex
}

func (r *DefaultMessageReceiver) SetConn(c *Conn) MessageReceiver {
	r.conn = c
	return r
}

func (r *DefaultMessageReceiver) GetConn() *Conn {
	return r.conn
}

func (r *DefaultMessageReceiver) ReadWhole(maxMsgDataLen uint64) (msg *Message, err error) {
	defer r.mu.Unlock()
	r.mu.Lock()

	if r.conn.GetState() != StateOpen {
		return nil, ErrConnIsNotOpen
	}

	msg = &Message{}

	frame := &Frame{}
	if err := frame.FromBufReader(r.conn.Buf, maxMsgDataLen); err != nil {
		if err == ErrFrameTooLarge {
			return nil, ErrMessageTooLarge
		}
		return nil, err
	}

	msg.Opcode = frame.Opcode
	msg.Data = frame.PayloadData

	if frame.FIN == 1 {
		return msg, nil
	}

	var msgLen uint64
	msgLen += frame.PayloadLen

	for {
		if r.conn.GetState() != StateOpen {
			return nil, ErrConnIsNotOpen
		}

		if err := frame.FromBufReader(r.conn.Buf, maxMsgDataLen); err != nil {
			if err == ErrFrameTooLarge {
				return nil, ErrMessageTooLarge
			}
			return nil, err
		}

		msgLen += frame.PayloadLen
		if msgLen > maxMsgDataLen {
			return nil, ErrMessageTooLarge
		}

		msg.Data = append(msg.Data, frame.PayloadData...)
		if frame.FIN == 1 {
			return msg, nil
		}
	}

	return nil, nil
}

func (r *DefaultMessageReceiver) BeginReadFrame() {
	r.mu.Lock()
}

func (r *DefaultMessageReceiver) EndReadFrame() {
	r.mu.Unlock()
}

func (r *DefaultMessageReceiver) ReadFrame(maxFramePayloadLen uint64) (frame *Frame, fin bool, err error) {
	if r.conn.GetState() != StateOpen {
		return nil, false, ErrConnIsNotOpen
	}

	frame = &Frame{}
	if err := frame.FromBufReader(r.conn.Buf, maxFramePayloadLen); err != nil {
		return nil, false, err
	}

	return frame, frame.FIN == 1, nil
}

func (r *DefaultMessageReceiver) IsConnOpen() bool {
	return r.conn.GetState() == StateOpen
}

type BufReader interface {
	io.Reader
	io.ByteScanner
}

type MessageSender interface {
	SetConn(c *Conn) MessageSender
	GetConn() *Conn

	SendWhole(msg *Message, mask bool) (n int, err error)
	SendWholeWithReader(r io.Reader, opcode uint8, mask bool) (n int, err error)
	SendWholeBytes(byts []byte, mask bool) (n int, err error)

	BeginSendFrame()
	SendFrame(data []byte, opcode uint8, begin bool, end bool, mask bool) (n int, err error)
	SendFrameWithReader(r BufReader, opcode uint8, perFrameSize int, mask bool) (n int, err error)
	EndSendFrame()

	SendClose(code uint16, reason string, useCodeText bool, mask bool)
	IsConnOpen() bool
}

type DefaultMessageSender struct {
	conn *Conn
	mu   sync.Mutex
}

func (s *DefaultMessageSender) SetConn(c *Conn) MessageSender {
	s.conn = c
	return s
}

func (s *DefaultMessageSender) GetConn() *Conn {
	return s.conn
}

func (s *DefaultMessageSender) SendWhole(msg *Message, mask bool) (n int, err error) {
	defer s.mu.Unlock()
	s.mu.Lock()

	if s.conn.GetState() != StateOpen {
		return 0, ErrConnIsNotOpen
	}

	frame := &Frame{}
	frame.FIN = 1
	frame.Opcode = msg.Opcode
	frame.PayloadData = msg.Data

	return frame.WriteTo(s.conn, mask)
}

func (s *DefaultMessageSender) SendWholeBytes(byts []byte, mask bool) (n int, err error) {
	msg := &Message{}
	msg.Opcode = OpcodeText
	msg.Data = byts

	return s.SendWhole(msg, mask)
}

func (s *DefaultMessageSender) SendWholeWithReader(r io.Reader, opcode uint8, mask bool) (n int, err error) {
	defer s.mu.Unlock()
	s.mu.Lock()

	if s.conn.GetState() != StateOpen {
		return 0, ErrConnIsNotOpen
	}

	data := make([]byte, 512)
	buf := make([]byte, 512)
	for {
		i, err := r.Read(buf)

		if i > 0 {
			data = append(data, buf[:i]...)
		}

		if err != nil {
			if err == io.EOF {
				break
			} else {
				return 0, err
			}
		}
	}

	frame := &Frame{}
	frame.FIN = 1
	frame.Opcode = opcode
	frame.PayloadData = data

	return frame.WriteTo(s.conn, mask)
}

func (s *DefaultMessageSender) BeginSendFrame() {
	s.mu.Lock()
}

func (s *DefaultMessageSender) EndSendFrame() {
	s.mu.Unlock()
}

func (s *DefaultMessageSender) SendFrame(data []byte, opcode uint8, begin bool, end bool, mask bool) (n int, err error) {
	if s.conn.GetState() != StateOpen {
		return 0, ErrConnIsNotOpen
	}

	frame := &Frame{}

	if begin {
		frame.Opcode = opcode
	} else {
		frame.Opcode = OpcodeContinue
	}

	if end {
		frame.FIN = 1
	}

	frame.PayloadData = data
	return frame.WriteTo(s.conn, mask)
}

func (s *DefaultMessageSender) SendFrameWithReader(r BufReader, opcode uint8, perFrameSize int, mask bool) (n int, err error) {
	if s.conn.GetState() != StateOpen {
		return 0, ErrConnIsNotOpen
	}

	buf := make([]byte, perFrameSize)

	si := 0
	begin := true
	end := false

	for {
		i, err := r.Read(buf)

		if i > 0 {
			// read one more byte to see if EOF is coming up
			_, pre := r.ReadByte()
			if pre == io.EOF {
				end = true
			} else {
				r.UnreadByte()
			}

			if si, err = s.SendFrame(buf[:i], opcode, begin, end, mask); err != nil {
				return 0, err
			}

			n += si
			begin = false

			if pre == io.EOF {
				return n, nil
			}
		}

		if err != nil {
			if err == io.EOF {
				return n, nil
			}
			return 0, err
		}
	}
}

func (s *DefaultMessageSender) SendClose(code uint16, reason string, useCodeText bool, mask bool) {
	s.conn.SetState(StateClosed)

	frame := MakeCloseFrame(code, reason, useCodeText)
	frame.WriteTo(s.conn, mask)

	s.conn.Close()
}

func (s *DefaultMessageSender) IsConnOpen() bool {
	return s.conn.GetState() == StateOpen
}
