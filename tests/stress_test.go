package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

func TestStress(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", ":11451")
	if err != nil {
		panic(err)
	}

	simultaneosClients := 60
	duration := 20 * time.Second

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

			payload := make([]byte, 400)
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

func TestLatency(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", "rowlet-lp.ddns.net:11451")
	if err != nil {
		panic(err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}

	ping := 83
	average := 0.
	for range ping {
	conn.Write([]byte{0x02, 0x01, 0x01, 0x01})
	now := time.Now()

	conn.ReadFrom([]byte{})

	delay := time.Since(now).Milliseconds()
	fmt.Printf("Took %dms seconds to respond\n", delay)
	average += float64(delay)/float64(ping)
	}
	fmt.Printf("Average: %fms", average)
}