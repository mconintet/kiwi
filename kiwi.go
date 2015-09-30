package kiwi

import (
	"crypto/sha1"
	"encoding/base64"
	"errors"
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
