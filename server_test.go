package kiwi

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"testing"
)

func TestEcho(t *testing.T) {
	srv := NewServer()
	srv.Addr, _ = net.ResolveTCPAddr("tcp", ":9876")

	srv.ApplyDefaultCfg()

	srv.OnConnOpenFunc("/", func(r MessageReceiver, s MessageSender) {
		for {
			msg, err := r.ReadWhole(1 << 20)

			if err != nil {
				log.Println(err)
				s.SendClose(CloseCodeGoingAway, "", true, false)
				break
			}

			log.Printf("new message with opcode:%d", msg.Opcode)
			
			if msg.IsText() {
				msgText := string(msg.Data)
				log.Println(msgText)

				if msgText == "close" {
					s.SendClose(CloseCodeGoingAway, "", true, false)
				} else {
					s.SendWhole(msg, false)
				}
			} else if msg.IsClose() {
				s.SendClose(CloseCodeNormalClosure, "", true, false)
				log.Println("closed")
				break
			}
		}
	})

	srv.ListenAndServe()
}

func TestEchoFrame(t *testing.T) {
	srv := NewServer()
	srv.Addr, _ = net.ResolveTCPAddr("tcp", ":9876")

	srv.ApplyDefaultCfg()

	srv.OnConnOpenFunc("/frame", func(r MessageReceiver, s MessageSender) {
		for {
			msg, err := r.ReadWhole(1 << 20)

			if err != nil {
				log.Println(err)
				s.SendClose(CloseCodeGoingAway, "", true, false)
				break
			}
			
			log.Printf("new message with opcode:%d", msg.Opcode)

			if msg.IsText() {
				msgText := string(msg.Data)
				log.Println(msgText)

				if msgText == "close" {
					s.SendClose(CloseCodeGoingAway, "", true, false)
				} else {
					buf := bytes.NewBuffer(msg.Data)
					s.BeginSendFrame()
					s.SendFrameWithReader(buf, OpcodeText, 20, false)
					s.EndSendFrame()
				}
			} else if msg.IsClose() {
				s.SendClose(CloseCodeNormalClosure, "", true, false)
				log.Println("closed")
				break
			}
		}
	})

	srv.ListenAndServe()
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

func TestUnicode2utf8(t *testing.T) {
	u8, _ := Unicode2utf8(0x75)
	if !reflect.DeepEqual(u8, []byte{0x75}) {
		t.Fatal("not pass 'u'")
	}

	u8, _ = Unicode2utf8(0xA9)
	if !reflect.DeepEqual(u8, []byte{0xC2, 0xA9}) {
		t.Fatal("not pass '©'")
	}

	u8, _ = Unicode2utf8(0x6C49)
	if !reflect.DeepEqual(u8, []byte{0xE6, 0xB1, 0x89}) {
		t.Fatal("not pass '汉'")
	}

	u8, _ = Unicode2utf8(0x1F604)
	if !reflect.DeepEqual(u8, []byte{0xF0, 0x9F, 0x98, 0x84}) {
		t.Fatal("not pass '😄'")
	}
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

func TestUtf82unicode(t *testing.T) {
	u, _ := Utf82unicode([]byte{0x75})
	if u != 0x75 {
		t.Fatal("not pass 'u'")
	}

	u, _ = Utf82unicode([]byte{0xC2, 0xA9})
	if u != 0xA9 {
		t.Fatal("not pass '©'")
	}

	u, _ = Utf82unicode([]byte{0xE6, 0xB1, 0x89})
	if u != 0x6C49 {
		t.Fatalf("not pass '汉': %x", u)
	}

	u, _ = Utf82unicode([]byte{0xF0, 0x9F, 0x98, 0x84})
	if u != 0x1F604 {
		t.Fatal("not pass '😄'")
	}
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

type ValidTest struct {
	in  string
	out bool
}

var validTests = []ValidTest{
	{"", true},
	{"a", true},
	{"abc", true},
	{"Ж", true},
	{"ЖЖ", true},
	{"брэд-ЛГТМ", true},
	{"☺☻☹", true},
	{string([]byte{66, 250}), false},
	{string([]byte{66, 250, 67}), false},
	{"a\uFFFDb", true},
	{string("\xF4\x8F\xBF\xBF"), true},      // U+10FFFF
	{string("\xF4\x90\x80\x80"), false},     // U+10FFFF+1; out of range
	{string("\xF7\xBF\xBF\xBF"), false},     // 0x1FFFFF; out of range
	{string("\xFB\xBF\xBF\xBF\xBF"), false}, // 0x3FFFFFF; out of range
	{string("\xc0\x80"), false},             // U+0000 encoded in two bytes: incorrect
	{string("\xed\xa0\x80"), false},         // U+D800 high surrogate (sic)
	{string("\xed\xbf\xbf"), false},         // U+DFFF low surrogate (sic)
}

func TestIsIntactUtf8(t *testing.T) {
	for i, tt := range validTests {
		if IsIntactUtf8([]byte(tt.in)) != tt.out {
			t.Fatalf("[CASE %d] IsIntactUtf8(%q) = %v; want %v", i, tt.in, !tt.out, tt.out)
		}
	}
}


type makeAcceptKeyTest struct {
	in string
	out string
}

var makeAcceptKeyTests = []makeAcceptKeyTest{
	{"M/A=","5oBJ6efz0YUYE2VFXcCfYKTBqYY="},
}
func TestMakeAcceptKey(t *testing.T){
	for _, tt := range makeAcceptKeyTests {
		if MakeAcceptKey(tt.in) != tt.out {
			t.Fatalf("request key: %s with: %s got: %s", tt.in, MakeAcceptKey(tt.in), tt.out)
		}
	}
}
