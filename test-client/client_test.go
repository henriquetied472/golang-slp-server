package main

import (
	"context"
	"crypto/rand"
	"net"
	"sync"
	"testing"
	"time"
)

func TestStress(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "rowlet-lp.ddns.net:39158")
	
	simultaneosClients := 7
	duration := 25 * time.Second

	wg := sync.WaitGroup{}
	wg.Add(simultaneosClients)
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	for range simultaneosClients {
		go func() {
			conn, err := net.DialUDP("udp", nil, addr)
			if err != nil {
				panic(err)
			}

			payload := make([]byte, 1400)
			rand.Read(payload)
			payload[0] = 0x01
			payload[12], payload[13] = 0x0a, 0x0d
			payload[16], payload[17] = 0x0a, 0x0d
			for {
				conn.Write(payload)
				select {
				case <-ctx.Done():
					wg.Done()
					return
				default:
				}
			}
		}()
	}

	wg.Wait()
}