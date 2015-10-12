package kiwi

import (
	"bytes"
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
	in  string
	out string
}

var makeAcceptKeyTests = []makeAcceptKeyTest{
	{"M/A=", "5oBJ6efz0YUYE2VFXcCfYKTBqYY="},
}

func TestMakeAcceptKey(t *testing.T) {
	for _, tt := range makeAcceptKeyTests {
		if MakeAcceptKey(tt.in) != tt.out {
			t.Fatalf("request key: %s with: %s got: %s", tt.in, MakeAcceptKey(tt.in), tt.out)
		}
	}
}
