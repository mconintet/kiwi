package kiwi

import (
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const (
	keySuffixGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

type ServerError struct {
	ErrorString string
}

func (se *ServerError) Error() string {
	return se.ErrorString
}

type ProtocolError struct {
	ErrorString string
}

func (pe *ProtocolError) Error() string {
	return pe.ErrorString
}

func MakeAcceptKey(reqKey string) string {
	h := sha1.New()
	h.Write([]byte(reqKey + keySuffixGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

const (
	CloseCodeNormalClosure           = uint16(1000)
	CloseCodeGoingAway               = uint16(1001)
	CloseCodeProtocolError           = uint16(1002)
	CloseCodeUnsupportedData         = uint16(1003)
	CloseCodeNoStatusRcvd            = uint16(1005)
	CloseCodeAbnormalClosure         = uint16(1006)
	CloseCodeInvalidFramePayloadData = uint16(1007)
	CloseCodePolicyViolation         = uint16(1008)
	CloseCodeMessageTooBig           = uint16(1009)
	CloseCodeMandatoryExt            = uint16(1010)
	CloseCodeInternalServerError     = uint16(1011)
	CloseCodeTLSHandshake            = uint16(1015)
)

var closeCodeText = map[uint16]string{
	CloseCodeNormalClosure:           "Normal Closure",
	CloseCodeGoingAway:               "Going Away",
	CloseCodeProtocolError:           "Protocol error",
	CloseCodeUnsupportedData:         "Unsupported Data",
	CloseCodeNoStatusRcvd:            "No Status Rcvd",
	CloseCodeAbnormalClosure:         "Abnormal Closure",
	CloseCodeInvalidFramePayloadData: "Invalid frame payload data",
	CloseCodePolicyViolation:         "Policy Violation",
	CloseCodeMessageTooBig:           "Message Too Big",
	CloseCodeMandatoryExt:            "Mandatory Ext",
	CloseCodeInternalServerError:     "Internal Server Error",
	CloseCodeTLSHandshake:            "TLS handshake",
}

func CloseCodeText(code uint16) string {
	return closeCodeText[code]
}

func ReadBytesAsMath(r io.Reader, size uint64) (byts []byte, err error) {
	buf := make([]byte, 512)
	var n uint64

	for {
		i, err := r.Read(buf)

		if i > 0 {
			n += uint64(i)
			byts = append(byts, buf[:i]...)

			if n == size {
				return byts, nil
			}
		}

		if err != nil {
			return nil, errors.New("buf not enough")
		}
	}
}

func Unicode2utf8(u uint32) (u8 []byte, err error) {
	if u <= 0x7F {
		return []byte{byte(u)}, nil
	} else if u >= 0x80 && u <= 0x7FF {
		return []byte{
			byte(u>>6 | 0xC0),
			byte(u&0x3F | 0x80),
		}, nil
	} else if u >= 0x800 && u <= 0xFFFF {
		return []byte{
			byte(u>>12 | 0xE0),
			byte((u&0xFC0)>>6 | 0x80),
			byte(u&0x3F | 0x80),
		}, nil
	} else if u >= 0x10000 && u <= 0x10FFFF {
		return []byte{
			byte(u>>18 | 0xF0),
			byte((u&0x3F000)>>12 | 0x80),
			byte((u&0xFC0)>>6 | 0x80),
			byte(u&0x3F | 0x80),
		}, nil
	}

	return nil, errors.New(fmt.Sprintf("deformed unicode: %d", u))
}

func Utf82unicode(u8 []byte) (u uint32, err error) {
	u8l := len(u8)

	if u8l == 0 {
		return 0, errors.New("empty utf8")
	}

	b1 := u8[0]
	if b1 <= 0x7F {
		return uint32(b1), nil
	} else if b1>>5 == 0x6 && u8l == 2 {
		return uint32(b1&0x1F)<<6 |
			uint32(u8[1]&0x3F), nil
	} else if b1>>4 == 0xE && u8l == 3 {
		return uint32(b1&0xF)<<12 |
			uint32(u8[1]&0x3F)<<6 |
			uint32(u8[2]&0x3F), nil

	} else if b1>>3 == 0x1E && u8l == 4 {
		return uint32(b1&0x7)<<18 |
			uint32(u8[1]&0x3F)<<12 |
			uint32(u8[2]&0x3F)<<6 |
			uint32(u8[3]&0x3F), nil
	}

	return 0, errors.New(fmt.Sprintf("deformed utf8: %d", u8))
}

func IsIntactUtf8(u8 []byte) bool {
	i := 0
	u8l := len(u8)

	for {
		if i == u8l {
			break
		}

		b1 := u8[i]
		var tu uint32

		switch {
		case b1 <= 0x7F:
		case b1>>5 == 0x6:
			if u8l-i >= 2 &&
				u8[i+1]&0xC0 == 0x80 &&
				// U+0000 encoded in two bytes: incorrect
				(u8[i] > 0xC0 || u8[i+1] > 0x80) {
				i++
			} else {
				return false
			}
		case b1>>4 == 0xE:
			if u8l-i >= 3 {
				tu = uint32(b1&0xF)<<12 |
					uint32(u8[i+1]&0x3F)<<6 |
					uint32(u8[i+2]&0x3F)

				// UTF-8 prohibits encoding character numbers between U+D800 and U+DFFF
				if tu >= 0x800 && tu <= 0xFFFF && !(tu >= 0xD800 && tu <= 0xDFFF) {
					i += 2
				} else {
					return false
				}
			} else {
				return false
			}
		case b1>>3 == 0x1E:
			if u8l-i >= 4 &&
				u8[i]&0x7 <= 0x4 &&
				u8[i+1]&0xC0 == 0x80 && u8[i+1]&0x3F <= 0xF &&
				u8[i+2]&0xC0 == 0x80 &&
				u8[i+3]&0xC0 == 0x80 {
				i += 3
			} else {
				return false
			}
		default:
			return false
		}
		i++
	}

	return i == u8l
}
