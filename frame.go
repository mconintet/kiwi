package kiwi

import (
	"errors"
	"io"
	"math"
	"math/rand"
)

// 0                   1                   2                   3
// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-------+-+-------------+-------------------------------+
// |F|R|R|R| opcode|M| Payload len |    Extended payload length    |
// |I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
// |N|V|V|V|       |S|             |   (if payload len==126/127)   |
// | |1|2|3|       |K|             |                               |
// +-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
// |     Extended payload length continued, if payload len == 127  |
// + - - - - - - - - - - - - - - - +-------------------------------+
// |                               |Masking-key, if MASK set to 1  |
// +-------------------------------+-------------------------------+
// | Masking-key (continued)       |          Payload Data         |
// +-------------------------------- - - - - - - - - - - - - - - - +
// :                     Payload Data continued ...                :
// + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
// |                     Payload Data continued ...                |
// +---------------------------------------------------------------+

const (
	OpcodeContinue = uint8(0x0)
	OpcodeText     = uint8(0x1)
	OpcodeBinary   = uint8(0x2)
	OpcodeClose    = uint8(0x8)
	OpcodePing     = uint8(0x9)
	OpcodePong     = uint8(0xA)
)

type Frame struct {
	FIN    uint8
	RSV1   uint8
	RSV2   uint8
	RSV3   uint8
	Opcode uint8
	MASK   uint8

	PayloadLen uint64

	MaskingKey  uint32
	PayloadData []byte
}

var (
	ErrDeformedFirstTwoBytes         = &ProtocolError{"deformed first two bytes of frame"}
	ErrDeformedOpcode                = &ProtocolError{"deformed opcode"}
	ErrDeformedExtendedPayloadLength = &ProtocolError{"deformed extended payload length"}
	ErrDeformedMaskingKey            = &ProtocolError{"deformed masking key"}
	ErrDeformedPayloadData           = &ProtocolError{"deformed payload data"}
	ErrFrameTooLarge                 = errors.New("frame too large")
)

func CheckOpcode(code uint8) bool {
	return code == OpcodeContinue ||
		code == OpcodeText ||
		code == OpcodeBinary ||
		code == OpcodeClose ||
		code == OpcodePing ||
		code == OpcodePing ||
		code == OpcodePong
}

func (f *Frame) FromBufReader(r io.Reader, maxPayloadLen uint64) error {
	byt2 := make([]byte, 2)
	i, err := r.Read(byt2)
	if err != nil || i != 2 {
		return ErrDeformedFirstTwoBytes
	}

	f.FIN = byt2[0] >> 7
	f.RSV1 = (byt2[0] << 1) >> 7
	f.RSV2 = (byt2[0] << 2) >> 7
	f.RSV3 = (byt2[0] << 3) >> 7
	f.Opcode = byt2[0] & 0xF

	if !CheckOpcode(f.Opcode) {
		return ErrDeformedOpcode
	}

	f.MASK = byt2[1] >> 7
	pLen := byt2[1] & 0x7F

	if pLen <= 125 {
		f.PayloadLen = uint64(pLen)
	} else if pLen == 126 {
		var (
			p16  uint16
			byt2 = make([]byte, 2)
		)

		i, err := r.Read(byt2)
		if err != nil || i != 2 {
			return ErrDeformedExtendedPayloadLength
		}

		p16 = (uint16(byt2[0]) << 8) | uint16(byt2[1])
		f.PayloadLen = uint64(p16)
	} else if pLen == 127 {
		var (
			p64  uint64
			byt8 = make([]byte, 8)
		)

		i, err := r.Read(byt8)
		if err != nil || i != 8 {
			return ErrDeformedExtendedPayloadLength
		}

		p64 = uint64(byt8[0])<<56 |
			uint64(byt8[1])<<48 |
			uint64(byt8[2])<<40 |
			uint64(byt8[3])<<32 |
			uint64(byt8[4])<<24 |
			uint64(byt8[5])<<16 |
			uint64(byt8[6])<<8 |
			uint64(byt8[7])

		f.PayloadLen = p64
	} else {
		return &ProtocolError{"deformed payload length"}
	}

	if f.PayloadLen > maxPayloadLen {
		return ErrFrameTooLarge
	}

	var mkb []byte
	if f.MASK == 1 {
		mkb = make([]byte, 4)
		i, err := r.Read(mkb)
		if err != nil || i != 4 {
			return ErrDeformedMaskingKey
		}

		f.MaskingKey = uint32(mkb[0])<<24 |
			uint32(mkb[1])<<16 |
			uint32(mkb[2])<<8 |
			uint32(mkb[3])
	}

	if pLen > 0 {
		pld, err := ReadBytesAsMath(r, f.PayloadLen)
		if err != nil {
			return ErrDeformedPayloadData
		}

		if f.MASK == 1 {
			MaskData(pld, mkb)
		}

		f.PayloadData = pld
	}

	return nil
}

func (f *Frame) ToBytes(mask bool) (byts []byte, err error) {
	byts = make([]byte, 2)
	byts[0] = byte(f.FIN<<7 | f.RSV1<<6 | f.RSV2<<5 | f.RSV3<<4 | f.Opcode)

	pLength := len(f.PayloadData)
	pLen := 0
	var extPLen []byte

	if pLength <= 125 {
		pLen = pLength
	} else if pLength <= math.MaxUint16 {
		pLen = 126
		extPLen = []byte{
			byte(pLength >> 8),
			byte(pLength),
		}
	} else if uint64(pLength) <= math.MaxUint64 {
		pLen = 127
		extPLen = []byte{
			byte(pLength >> 56),
			byte(pLength >> 48),
			byte(pLength >> 40),
			byte(pLength >> 32),
			byte(pLength >> 24),
			byte(pLength >> 16),
			byte(pLength >> 8),
			byte(pLength),
		}
	} else {
		return nil, errors.New("too large frame")
	}

	if mask {
		f.MASK = uint8(1)
	}
	byts[1] = byte(f.MASK<<7 | uint8(pLen))

	if extPLen != nil {
		byts = append(byts, extPLen...)
	}

	if mask {
		mkb := MakeMaskingKey()
		byts = append(byts, mkb...)
	}

	byts = append(byts, f.PayloadData...)
	return byts, nil
}

func (f *Frame) WriteTo(w io.Writer, mask bool) (n int, err error) {
	if byts, err := f.ToBytes(mask); err != nil {
		return 0, err
	} else {
		return w.Write(byts)
	}
}

func MakeCloseFrame(code uint16, reason string, useCodeText bool) *Frame {
	if reason == "" && useCodeText {
		reason = CloseCodeText(code)
	}

	f := &Frame{}
	f.FIN = uint8(1)
	f.Opcode = OpcodeClose
	f.PayloadData = []byte{byte(code >> 8), byte(code)}
	f.PayloadData = append(f.PayloadData, reason...)

	return f
}

func MakeMaskingKey() []byte {
	r := rand.New(rand.NewSource(35))
	mk := r.Uint32()

	return []byte{
		byte(mk >> 24),
		byte(mk >> 16),
		byte(mk >> 8),
		byte(mk),
	}
}

func MaskData(data, maskingKey []byte) {
	for i := 0; i < len(data); i++ {
		j := i % 4
		data[i] = data[i] ^ maskingKey[j]
	}
}
