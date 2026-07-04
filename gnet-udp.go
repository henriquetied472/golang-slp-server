package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/panjf2000/gnet/v2"
)

type GnetPeer struct {
	User *struct {
		Username, Key string
	}
	Challenge []byte
	RInfo     *net.UDPAddr
	ExpireAt  time.Time
	Conn      gnet.Conn
}

var client *gnet.Client
var timeout time.Duration = 30 // seconds

type GnetPeerManager map[uint64]*GnetPeer

func (pm GnetPeerManager) GetPeer(addr *net.UDPAddr, conn gnet.Conn) *GnetPeer {
	uintAddr := UPDAddrToUint64(addr)
	if _, ok := pm[uintAddr]; !ok {
		pm[uintAddr] = &GnetPeer{RInfo: addr, Conn: conn}
	}
	pm[uintAddr].ExpireAt = time.Now().Add(timeout * time.Second)
	return pm[uintAddr]
}

func (pm GnetPeerManager) DeletePeer(addr *net.UDPAddr) {
	uintAddr := UPDAddrToUint64(addr)
	pm[uintAddr].Conn.Close()
	delete(pm, uintAddr)
}

func Tidy[Uint uint64 | uint32](pm map[Uint]*GnetPeer, isIpTable bool) {
	var tydyType string
	if isIpTable {
		tydyType = "IPTable"
	} else {
		tydyType = "GnetPeerManager"
	}

	mux.Lock()
	for addr, peer := range pm {
		if peer.ExpireAt.Compare(time.Now()) < 0 {
			slog.Debug("TidyPeers: Deleting expired peer at "+tydyType+":", "addr", peer.RInfo.String())
			peer.Conn.Close()
			delete(pm, addr)
		}
	}
	mux.Unlock()
}

type GnetSLPServer struct {
	*gnet.BuiltinEventEngine

	GnetPeerManager
	IPTable         map[uint32]*GnetPeer
	UploadLastSec   atomic.Uint64
	DownloadLastSec atomic.Uint64
	AuthProvider
}

func UPDAddrToUint64(addr *net.UDPAddr) uint64 {
	ip := addr.IP.To4()
	if ip == nil {
		slog.Error("UDPAddrToUint64: addr is not Ipv4")
		return 0
	}

	uintAddr := uint64(addr.Port) | uint64(ip[3])<<16 | uint64(ip[2])<<24 | uint64(ip[1])<<32 | uint64(ip[0])<<40
	return uintAddr
}

func NewGnetSLPServer(port int, auth AuthProvider) *GnetSLPServer {
	return &GnetSLPServer{
		GnetPeerManager: make(GnetPeerManager),
		IPTable:         make(map[uint32]*GnetPeer),
		UploadLastSec:   atomic.Uint64{},
		DownloadLastSec: atomic.Uint64{},
		AuthProvider:    auth,
	}
}

func (srv *GnetSLPServer) OnTraffic(conn gnet.Conn) gnet.Action {
	addr := conn.RemoteAddr().(*net.UDPAddr)
	msg, err := conn.Next(-1)
	if err != nil {
		slog.Error("OnTraffic: error while trying to read message:", "error", err.Error())
		return gnet.None
	}

	srv.DownloadLastSec.Add(uint64(len(msg)))

	fwdType := FowarderType(msg[0] & 0x7f)
	payload := msg[1:]

	mux.Lock()
	peer := srv.GetPeer(addr, conn)
	mux.Unlock()

	if fwdType != Keepalive || !ignoreKeepaliveDebug {
		slog.Debug("OnTraffic: New message:", "rinfo", addr.String(), "type", FowarderTypeName[fwdType], "size", len(payload))
	}

	if srv.AuthProvider != nil && peer.User == nil {
		srv.OnNeedAuth(peer, fwdType, payload)
	}

	switch fwdType {
	case Ipv4:
		srv.OnIpv4(peer, payload)
	case Ping:
		srv.OnPing(peer, msg)
	case Ipv4Frag:
		srv.OnPing(peer, payload)
	}

	return gnet.None
}

func (srv *GnetSLPServer) OnTick() (delay time.Duration, action gnet.Action) {
	Tidy(srv.GnetPeerManager, false)
	Tidy(srv.IPTable, true)

	fmt.Printf("\033[2KClients: %v | Upload: %vKB/s | Download: %vKB/s\r", srv.GetClientSize(), float64(srv.UploadLastSec.Load())/100, float64(srv.DownloadLastSec.Load())/100)

	srv.DownloadLastSec.Store(0)
	srv.UploadLastSec.Store(0)

	return 1, gnet.None
}

func (srv *GnetSLPServer) GetClientSize() int {
	return len(srv.GnetPeerManager)
}

func (srv *GnetSLPServer) OnNeedAuth(peer *GnetPeer, fwdType FowarderType, payload []byte) {

}

func (srv *GnetSLPServer) OnIpv4(peer *GnetPeer, payload []byte) {
	if len(payload) <= 20 { return }

	var src, dst uint32
	src = uint32(payload[12]) << 24 | uint32(payload[13]) << 16 | uint32(payload[14]) << 8 | uint32(payload[15])
	dst = uint32(payload[16]) << 24 | uint32(payload[17]) << 16 | uint32(payload[18]) << 8 | uint32(payload[19])

	slog.Debug("OnIpv4: New Ipv4 packet:", "from", fmt.Sprintf("%#.8x", src), "to", fmt.Sprintf("%#.8x", dst))

	mux.Lock()
	srv.IPTable[src] = peer
	dstPeer, ok := srv.IPTable[dst]
	mux.Unlock()
	if ok {
		slog.Debug("OnIpv4: destination ip is in cache")
		srv.Send(dstPeer, Ipv4, payload, false)
	} else {
		slog.Debug("OnIpv4: destination ip is not in cache")
		srv.Broadcast(peer, Ipv4, payload)
	}
}

func (srv *GnetSLPServer) OnPing(peer *GnetPeer, msg []byte) {
	srv.Send(peer, Ping, msg[:4], true)
}

func (srv *GnetSLPServer) OnIpv4Frag(peer *GnetPeer, payload []byte) {

}

func (srv *GnetSLPServer) Send(peer *GnetPeer, fwdType FowarderType, msg []byte, raw bool) {
	if raw {
		_, err := peer.Conn.Write(msg)
		if err != nil {
			slog.Error("GnetSLPServer.Send: error while trying to send raw message:", "rinfo", peer.RInfo.String(), "type", FowarderTypeName[fwdType], "err", err.Error())
		}
		return
	}

	n, err := peer.Conn.Write(append([]byte{byte(fwdType)}, msg...))
	if err != nil {
		slog.Error("GnetSLPServer.Send: error while trying to send message:", "rinfo", peer.RInfo.String(), "type", FowarderTypeName[fwdType], "err", err.Error())
	}
	slog.Debug("Send: Wrote", "size", n, "to", peer.RInfo.String())
}

func (srv *GnetSLPServer) Broadcast(except *GnetPeer, fwdType FowarderType, msg []byte) {
	mux.Lock()
	for addr, peer := range srv.GnetPeerManager {
		if addr == UPDAddrToUint64(except.RInfo) { continue }
		slog.Debug("Broadcast: Sending a message:", "from", except.RInfo.String(), "to", peer.RInfo.String(), "size", len(msg)+1)
		srv.Send(peer, fwdType, msg, false)
	}
	mux.Unlock()
}

func (srv *GnetSLPServer) Run(ctx context.Context) {
	var err error
	client, err = gnet.NewClient(srv, gnet.WithLogger(&Logger{Level: InfoLevel}))
	if err != nil {
		panic(err)
	}
	client.Start()

	log.Fatalln(gnet.Run(srv, "udp://:"+fmt.Sprint(port), gnet.WithTicker(true), gnet.WithMulticore(multicore), gnet.WithLogger(&Logger{Level: InfoLevel})))
}

type Logger struct {
	Level LogLevel
}

type LogLevel int

const (
	DebugLevel LogLevel = iota -1
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

func (logger *Logger) Debugf(format string, args ...any) {
	if logger.Level > DebugLevel {return}
	slog.Debug(fmt.Sprintf("GNet: " + format, args...))
}
func (logger *Logger) Infof(format string, args ...any) {
	if logger.Level > InfoLevel {return}
	slog.Info(fmt.Sprintf("GNet: " + format, args...))
}
func (logger *Logger) Warnf(format string, args ...any) {
	if logger.Level > WarnLevel {return}
	slog.Warn(fmt.Sprintf("GNet: " + format, args...))
}
func (logger *Logger) Errorf(format string, args ...any) {
	if logger.Level > ErrorLevel {return}
	slog.Error(fmt.Sprintf("GNet: " + format, args...))
}
func (logger *Logger) Fatalf(format string, args ...any) {
	if logger.Level > FatalLevel {return}
	log.Fatalf("FATAL GNet: " + format, args...)
}
