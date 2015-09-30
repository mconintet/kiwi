package kiwi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Header map[string][]string

func (h Header) FromBytes(bs []byte, isCRLF bool) error {
	br := bytes.NewBuffer(bs)
	for {
		line, err := br.ReadBytes('\n')
		if err == io.EOF {
			break
		}

		kvSep := bytes.IndexByte(line, ':')
		if kvSep < 0 {
			return errors.New("deformed header: " + string(line))
		}

		var val string
		if isCRLF {
			val = string(line[kvSep+2 : len(line)-2])
		} else {
			val = string(line[kvSep+2 : len(line)-1])
		}

		key := string(line[0:kvSep])
		h[key] = append(h[key], val)
	}
	return nil
}

func (h Header) Get(key string) []string {
	return h[key]
}

func (h Header) GetOne(key string) string {
	return h[key][0]
}

func (h Header) HasKey(key string) bool {
	_, ok := h[key]
	return ok
}

func (h Header) HasKeyAndValEqual(key, val string) bool {
	if v, ok := h[key]; !ok {
		return false
	} else {
		return v[0] == val
	}
}

func (h Header) HasKeyAndValContains(key, val string) bool {
	if v, ok := h[key]; !ok {
		return false
	} else {
		return strings.Contains(v[0], val)
	}
}

func (h Header) WriteTo(w io.Writer) (err error) {
	for k, vs := range h {
		for _, v := range vs {
			if _, err = fmt.Fprintf(w, "%s: %v\r\n", k, v); err != nil {
				return err
			}
		}
	}
	return nil
}
