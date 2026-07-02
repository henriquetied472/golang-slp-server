package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"
)

var listener bool
var address string

func init() {
	flag.BoolVar(&listener, "l", false, "Listener mode")
	flag.Parse()
	address = flag.Arg(0)
}

func main() {
	FowarderTypeName := make([]string, 0x11)
	FowarderTypeName[0] = "Keepalive"
	FowarderTypeName[1] = "Ipv4"
	FowarderTypeName[2] = "Ping"
	FowarderTypeName[3] = "Ipv4Frag"
	FowarderTypeName[4] = "AuthMe"
	FowarderTypeName[0x10] = "Info"
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)

	go func() {
		<-ctx.Done()
		os.Exit(0)
	}()

	if !listener {
		addr, err := net.ResolveUDPAddr("udp", address)
		if err != nil {
			panic(err)
		}

		conn, err := net.DialUDP("udp", nil, addr)
		if err != nil {
			panic(err)
		}

		go func() {
			for {
				conn.Write([]byte{0x00})
				time.Sleep(10 * time.Second)
			}
		}()

		for {
			var fwdType int
			fmt.Print("Please enter the fowarder type: ")
			fmt.Scanf("%d\n", &fwdType)
			fmt.Printf("Selected %v fowrader type\n", FowarderTypeName[fwdType])

			var payload []byte
			fmt.Print("Please enter the payload: ")
			fmt.Scanf("%x\n", &payload)
			fmt.Printf("Payload is \"%#v\"\n", payload)

			var confirm rune
			fmt.Print("Send the pakcage? (y/n): ")
			fmt.Scanf("%c\n", &confirm)
			if confirm != 'y' && confirm != 'Y' {
				continue
			}

			n, err := conn.Write(append([]byte{byte(fwdType)}, payload...))
			if err != nil {
				panic(err)
			}
			fmt.Printf("Sent %v bytes to %v\n", n, addr.String())
		}
	} else {
		addr, err := net.ResolveUDPAddr("udp", address)
		if err != nil {
			panic(err)
		}

		conn, err := net.DialUDP("udp", nil, addr)
		if err != nil {
			panic(err)
		}

		go func() {
			for {
				conn.Write([]byte{0x00})
				time.Sleep(10 * time.Second)
			}
		}()

		fmt.Printf("Waiting for UDP packets from %v\n\n", addr.String())
		for {
			msg := make([]byte, 2048)
			size, rAddr, err := conn.ReadFrom(msg)
			if err != nil {
				panic(err)
			}

			fmt.Printf("Received %v bytes from %v with fowarder type %v\nPayload :%#v\n\n", size, rAddr.String(), FowarderTypeName[int(msg[0])], msg[:len(msg)-size])
		}
	}
}
