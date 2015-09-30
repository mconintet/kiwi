package kiwi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type HandshakeError struct {
	ErrorString string
}

func (err *HandshakeError) Error() string {
	return err.ErrorString
}

type HandshakeRequest struct {
	Method     string
	RequestURI string
	RequestURL *url.URL
	Proto      string
	ProtoVer   string
	Header     Header
}

var (
	emptyLine1 = []byte("\n\n")
	emptyLine2 = []byte("\r\n\r\n")
)

func checkLastEmptyLine(bs []byte) (isCRLF, ok bool) {
	bsLen := len(bs)
	if bsLen > 2 && bytes.Compare(bs[bsLen-2:], emptyLine1) == 0 {
		return false, true
	} else if bsLen > 4 && bytes.Compare(bs[bsLen-4:], emptyLine2) == 0 {
		return true, true
	} else {
		return false, false
	}
}

func parseRequestLine(bs []byte, isCRLF bool) (lineLen int, method, requestUri, proto, protoVer string, err error) {
	newline := bytes.IndexByte(bs, '\n')
	if newline < 0 {
		err = errors.New("missing newline")
		return
	}

	var line []byte
	if isCRLF {
		line = bs[0 : newline-1]
	} else {
		line = bs[0:newline]
	}

	s1 := bytes.IndexByte(line, ' ')
	s2 := bytes.IndexByte(line[s1+1:], ' ')
	if s1 < 0 || s2 < 0 {
		err = errors.New("deformed parts")
		return
	}
	s2 += s1 + 1

	p := line[s2+1:]
	ps := bytes.IndexByte(p, '/')
	if ps < 0 {
		err = errors.New("deformed proto")
		return
	}
	return newline, string(line[:s1]), string(line[s1+1 : s2]), string(p[:ps]), string(p[ps+1:]), nil
}

func (h *HandshakeRequest) ReadFrom(r io.Reader, maxSize int) error {
	// read one more byte to check if request is too large
	maxHandshakeBytes := maxSize + 1
	hs := make([]byte, maxHandshakeBytes)

	reqSize, err := r.Read(hs)
	if err != nil {
		return &HandshakeError{"unable to read handshake"}
	} else if reqSize == maxHandshakeBytes {
		return &HandshakeError{"too large handshake"}
	}

	hs = hs[0:reqSize]
	isCRLF, ok := checkLastEmptyLine(hs)
	if !ok {
		return &HandshakeError{"missing last empty line"}
	}

	// remove last empty line
	if isCRLF {
		hs = hs[:reqSize-2]
	} else {
		hs = hs[:reqSize-1]
	}

	reqLineLen, method, requestUri, proto, protoVer, err := parseRequestLine(hs, isCRLF)
	if err != nil {
		return &HandshakeError{"invalid request line: " + err.Error()}
	}

	header := make(Header, 5)
	if err := header.FromBytes(hs[reqLineLen+1:], isCRLF); err != nil {
		return &HandshakeError{err.Error()}
	}

	reqUrl, err := url.Parse(requestUri)
	if err != nil {
		return &HandshakeError{"deformed requestUri: " + requestUri}
	}

	h.Method = method
	h.Proto = proto
	h.ProtoVer = protoVer
	h.RequestURI = requestUri
	h.RequestURL = reqUrl
	h.Header = header

	return nil
}

type HandshakeResponse struct {
	StatusCode int
	Header     Header
}

func (h *HandshakeResponse) WriteTo(w io.Writer) (err error) {
	if _, err = fmt.Fprintf(w, "HTTP/1.1 %03d %s\r\n", h.StatusCode, http.StatusText(h.StatusCode)); err != nil {
		return err
	}

	if err = h.Header.WriteTo(w); err != nil {
		return err
	}
	return nil
}
