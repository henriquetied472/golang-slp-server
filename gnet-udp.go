package main

import (
	"log"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/panjf2000/gnet/v2"
)

type GnetPeer struct {
	User struct {
		Username, Key string
	}
	Challenge []byte
	RInfo *net.UDPAddr
	ExpireAt time.Time
	Conn gnet.Conn
}

var defaultClient, _ = gnet.NewClient(&gnet.BuiltinEventEngine{})
var timeout time.Duration = 20 // seconds

type GnetPeerManager map[uint64]*GnetPeer

func (pm GnetPeerManager) GetPeer(addr *net.UDPAddr) *GnetPeer {
	uintAddr := UPDAddrToUint64(addr)
	if _, ok := pm[uintAddr]; !ok {
		conn, err := defaultClient.Dial("udp", addr.String())
		if err != nil {
			slog.Error("GnetPeerManager.Get: couldn't stabilish a connection: ", "addr", addr.String(), "error", err.Error())
			return nil
		}
		pm[uintAddr] = &GnetPeer{RInfo: addr, Conn: conn}
	}
	pm[uintAddr].ExpireAt = time.Now().Add(timeout*time.Second)
	return pm[uintAddr]
}

func (pm GnetPeerManager) DeletePeer(addr *net.UDPAddr) {
	uintAddr := UPDAddrToUint64(addr)
	pm[uintAddr].Conn.Close()
	delete(pm, uintAddr)
}

func Tidy(pm map[uint64]*GnetPeer, isIpTable bool) {
	var tydyType string
	if isIpTable {
		tydyType = "IPTable"
	} else {
		tydyType = "GnetPeerManager"
	}

	for addr, peer := range pm {
		if peer.ExpireAt.Compare(time.Now()) < 0 {
			slog.Debug("TidyPeers: Deleting expired peer at "+tydyType+": ", "addr", peer.RInfo.String())
			peer.Conn.Close()
			delete(pm, addr)
		}
	}
}

type GnetSLPServer struct {
	gnet.BuiltinEventEngine

	GnetPeerManager
	IPTable map[uint64]*GnetPeer
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

	uintAddr := uint64(addr.Port) | uint64(ip[3]) << 16 | uint64(ip[2]) << 24 | uint64(ip[1]) << 32 | uint64(ip[0]) << 40
	return uintAddr
}

func NewGnetSLPServer(port int, auth AuthProvider) *GnetSLPServer {
	return &GnetSLPServer{
		GnetPeerManager: make(GnetPeerManager),
		IPTable: make(map[uint64]*GnetPeer),
		UploadLastSec: atomic.Uint64{},
		DownloadLastSec: atomic.Uint64{},
		AuthProvider: auth,
	}
}

func (srv *GnetSLPServer) OnTraffic(conn gnet.Conn) gnet.Action {
	return gnet.None
}

func (srv *GnetSLPServer) GetClientSize() int {
	return 0
}

func (srv *GnetSLPServer) Run() {
	slog.Info("GnetServer listening on ")
	log.Fatalln(gnet.Run(&GnetSLPServer{}, "udp://:11451"))
}