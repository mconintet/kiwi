package kiwi

import (
	"bytes"
	"log"
	"net"
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
				break
			}
		}
	})

	srv.ListenAndServe()
}
