package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
)

var httpAuth string
var jsonAuth string
var simpleAuth string
var port int
var debug bool
var ignoreKeepaliveDebug bool

func init() {
	flag.StringVar(&httpAuth, "httpAuth", "", "define HttpAuthProvider url")
	flag.StringVar(&jsonAuth, "jsonAuth", "", "define JsonAuthProvider file")
	flag.StringVar(&simpleAuth, "simpleAuth", "", "define CustomAuthProvider username and password (username:password)")
	flag.IntVar(&port, "port", 11451, "define server port")
	flag.BoolVar(&debug, "debug", false, "")
	flag.BoolVar(&ignoreKeepaliveDebug, "ikdebug", false, "ignore Keepalive debug messages")
	flag.Parse()
}

func main() {
	var provider AuthProvider
	if httpAuth != "" {
		provider = NewHttpAuthProvider(httpAuth)
	} else if jsonAuth != "" {
		provider = NewJsonAuthProvider(jsonAuth)
	} else if simpleAuth != "" {
		args := strings.Split(simpleAuth, ":")
		if len(args) != 2 {
			log.Fatalln("Bad value (use username:password)")
		}
	}

	if debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	udpServer := NewSLPServer(port, provider)
	udpServer.Run(ctx)
	RunMonitor(ctx, udpServer, port)

	<-ctx.Done()
	fmt.Print("\n")
}