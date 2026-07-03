package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/netip"

	// "strings"
	"sync/atomic"
	"time"
)

type FowarderType int

const (
	Keepalive FowarderType = iota
	Ipv4
	Ping
	Ipv4Frag
	AuthMe
	Info = 0x10

	OutputEncrypted = false
)

var FowarderTypeName = make([]string, 0x11)

func init() {
	FowarderTypeName[0] = "Keepalive"
	FowarderTypeName[1] = "Ipv4"
	FowarderTypeName[2] = "Ping"
	FowarderTypeName[3] = "Ipv4Frag"
	FowarderTypeName[4] = "Auth"
	FowarderTypeName[0x10] = "Info"
}

type CacheItem struct {
	*Peer
	ExpireAt time.Time
}

func ClearCache[T comparable](cache map[T]*CacheItem) {
	now := time.Now()
	for k, c := range cache {
		if c.ExpireAt.Compare(now) < 0 {
			slog.Debug("Expired cache item beign cleared", "key", k, "expire-at", c.ExpireAt.String(), "now", time.Now().String(), "difference", c.ExpireAt.Compare(time.Now()))
			delete(cache, k)
		}
	}
}

type Peer struct {
	*User
	Challenge []byte
	RInfo     net.Addr
}

type User struct {
	Username string
	Key      string
}

type PeerManager struct {
	Peers map[string]*CacheItem
}

func (pm *PeerManager) Delete(addr net.Addr) {
	delete(pm.Peers, addr.String())
}

func (pm *PeerManager) Get(addr net.Addr) *Peer {
	key := addr.String()
	peer, ok := pm.Peers[key]
	if !ok {
		pm.Peers[key] = &CacheItem{&Peer{RInfo: addr}, time.Now().Add(30 * time.Second)}
		peer = pm.Peers[key]
	} else {
		peer.ExpireAt = time.Now().Add(30 * time.Second)
	}
	return peer.Peer
}

func (pm *PeerManager) Tidy() {
	ClearCache(pm.Peers)
}

func (pm *PeerManager) GetUsersLogged() (count int) {
	for _, u := range pm.Peers {
		if u.Peer.User != nil {
			count++
		}
	}
	return count
}

func (pm *PeerManager) All(except net.Addr) []*Peer {
	peers := []*Peer{}
	for _, p := range pm.Peers {
		if p.Peer.RInfo == except {
			continue
		}
		peers = append(peers, p.Peer)
	}
	return peers
}

type SLPServer struct {
	IpCache map[uint32]*CacheItem
	*PeerManager
	UploadLastSec   atomic.Uint64
	DownloadLastSec atomic.Uint64
	AuthProvider
	Port     int
	SendChan chan Packet
}

type Packet struct {
	Msg  []byte
	Addr *net.UDPAddr
}

func NewSLPServer(port int, auth AuthProvider) *SLPServer {
	return &SLPServer{
		IpCache:      make(map[uint32]*CacheItem),
		PeerManager:  &PeerManager{Peers: make(map[string]*CacheItem)},
		AuthProvider: auth,
		Port:         port,
	}
}

func (server *SLPServer) GetClientSize() int {
	return len(server.PeerManager.Peers)
}

type Head struct {
	FowarderType
	IsEncrypted bool
}

func (server *SLPServer) ParseHead(msg []byte) *Head {
	return &Head{
		FowarderType: FowarderType(msg[0] & 0x7f),
		IsEncrypted:  msg[0]&0x80 != 0,
	}
}

func (server *SLPServer) OnMessage(msg []byte, rinfo net.Addr) {
	if len(msg) == 0 {
		return
	}

	server.DownloadLastSec.Add(uint64(len(msg)))

	head := server.ParseHead(msg)
	if head.FowarderType == Ping && !head.IsEncrypted {
		server.OnPing(rinfo, msg)
		return
	}
	
	peer := server.PeerManager.Get(rinfo)
	payload := msg[1:]
	
	if slog.Default().Enabled(context.Background(), slog.LevelDebug) && (ignoreKeepaliveDebug == (head.FowarderType!=0)) {
		var fwdTypeName string
		if int(head.FowarderType) < len(FowarderTypeName) {
			fwdTypeName = FowarderTypeName[head.FowarderType]
		} else {
			fwdTypeName = "Info"
		}

		slog.Debug("New message: ", "rinfo", rinfo.String(), "fowarder-type", fwdTypeName, "is-encrypted", head.IsEncrypted, "size", len(payload), "payload", fmt.Sprintf("%#v", payload))
	}

	if server.AuthProvider != nil {
		if peer.User == nil {
			server.OnNeedAuth(peer, head.FowarderType, payload)
			return
		}
	}
	server.OnPacket(peer, head.FowarderType, payload)
}

func (server *SLPServer) OnPacket(peer *Peer, fwdType FowarderType, payload []byte) {
	switch fwdType {
	case Keepalive:
	case Ipv4:
		server.OnIpv4(peer, payload)
	case Ping:
		slog.Error("encrypted ping?")
	case Ipv4Frag:
		server.OnIpv4Frag(peer, payload)
	}
}

func (server *SLPServer) SendInfo(peer *Peer, info string) {
	server.SendTo(peer, Info, []byte(info))
}

func (server *SLPServer) OnNeedAuth(peer *Peer, fwdType FowarderType, payload []byte) {
	if fwdType == AuthMe {
		if server.AuthProvider != nil && peer.Challenge != nil {
			if len(payload) <= 20 {
				return
			}

			response := payload[:20]
			username := string(payload[20:])
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result := make(chan bool)
			go func(ch chan<- bool) {
				ch <- server.AuthProvider.Verify(username, peer.Challenge[1:], response)
			}(result)

			correctPassword := false
			select {
			case res := <-result:
				correctPassword = res
			case <-ctx.Done():
				err = fmt.Errorf("login: authentication timeout")
			}

			if !correctPassword {
				err = fmt.Errorf("login: incorrect password")

			}

			if err != nil {
				slog.Error("%v | user: %v", err.Error(), username)
				server.SendInfo(peer, err.Error())
			}
		}
	} else {
		if peer.Challenge == nil {
			randBytes := make([]byte, 65)
			rand.Read(randBytes)
			randBytes[0] = 0
			peer.Challenge = randBytes
		}

		server.SendTo(peer, AuthMe, peer.Challenge)
	}
}

func (server *SLPServer) OnIpv4Frag(peer *Peer, payload []byte) {
	if len(payload) <= 20 {
		return
	}

	buf := bytes.NewBuffer(payload)
	var src, dst uint32
	binary.Read(buf, binary.BigEndian, &src)
	binary.Read(buf, binary.BigEndian, &dst)

	server.IpCache[src] = &CacheItem{peer, time.Now().Add(30 * time.Second)}

	if dstPeer, ok := server.IpCache[dst]; ok {
		server.SendTo(dstPeer.Peer, Ipv4Frag, payload)
	} else {
		server.SendBroadcast(peer, Ipv4Frag, payload)
	}
}

func (server *SLPServer) OnPing(rinfo net.Addr, msg []byte) {
	server.SendToRaw(rinfo, msg[:4])
}

func (server *SLPServer) OnIpv4(peer *Peer, payload []byte) {
	if len(payload) <= 20 {
		return
	}

	buf := bytes.NewBuffer(payload)
	var src, dst uint32
	buf.Next(12)
	binary.Read(buf, binary.BigEndian, &src)
	binary.Read(buf, binary.BigEndian, &dst)
	slog.Debug("Ipv4 event source: ", "ipv4", netip.AddrFrom4([4]byte(binary.BigEndian.AppendUint32([]byte{}, src))).String())
	slog.Debug("Ipv4 event destination: ", "ipv4", netip.AddrFrom4([4]byte(binary.BigEndian.AppendUint32([]byte{}, dst))).String())

	server.IpCache[src] = &CacheItem{peer, time.Now().Add(30 * time.Second)}

	if dstPeer, ok := server.IpCache[dst]; ok {
		server.SendTo(dstPeer.Peer, Ipv4, payload)
	} else {
		server.SendBroadcast(peer, Ipv4, payload)
	}
}

func (server *SLPServer) SendTo(peer *Peer, fwdType FowarderType, payload []byte) {
	if OutputEncrypted {
		slog.Warn("OutputEncrypted not implemented")
	}

	slog.Debug("Sended message: ", "rinfo", peer.RInfo.String(), "fowarder-type", fwdType, "size", len(payload), "payload", fmt.Sprintf("%#v", payload))
	server.SendToRaw(peer.RInfo, append([]byte{byte(fwdType)}, payload...))
}

func (server *SLPServer) SendToRaw(rinfo net.Addr, msg []byte) {
	addr, err := net.ResolveUDPAddr("udp", rinfo.String())
	if err != nil {
		slog.Error("UPD address resolving error: " + err.Error())
	}

	server.UploadLastSec.Add(uint64(len(msg)))
	server.SendChan <- Packet{msg, addr}
}

func (server *SLPServer) SendBroadcast(except *Peer, fwdType FowarderType, payload []byte) {
	for _, p := range server.PeerManager.All(except.RInfo) {
		server.SendTo(p, fwdType, payload)
	}
}

func (server *SLPServer) Tidy() {
	server.PeerManager.Tidy()
	ClearCache(server.IpCache)
}

func (server *SLPServer) Run(ctx context.Context) {
	server.SendChan = make(chan Packet, 20)

	if auth, ok := server.AuthProvider.(*JsonAuthProvider); ok {
		auth.Run(ctx)
	}

	go func() {
		conn, err := net.ListenUDP("udp", must(net.ResolveUDPAddr("udp", ":"+fmt.Sprint(server.Port))))
		if err != nil {
			log.Fatalf("[FATAL] Couldn't listen on %v: %v\f", conn.LocalAddr().String(), err)
		}
		defer conn.Close()

		buffer := make([]byte, 1500)

		slog.Info("Server listening on " + conn.LocalAddr().String())

	loop:
		for {
			n, remoteAddr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				slog.Error("Couldn't read packet: " + err.Error())
			}

			msg := buffer[:n]
			server.OnMessage(msg, remoteAddr)

			select {
			case packet := <-server.SendChan:
				n, err := conn.WriteToUDP(packet.Msg, packet.Addr)
				slog.Debug("Sending UDP Packet: ", "size", n, "addr", packet.Addr.String())
				if err != nil {
					slog.Debug("Sending UDP Packet failed")
					server.PeerManager.Delete(packet.Addr)
				}
			case <-ctx.Done():
				break loop
			default:
			}
		}

		slog.Info("Server Closed")
	}()

	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Printf("\033[2K\rClients: %v | Upload: %vKB/s | Download: %vKB/s", server.GetClientSize(), float64(server.UploadLastSec.Load())/100, float64(server.DownloadLastSec.Load())/100)
			server.DownloadLastSec.Store(0)
			server.UploadLastSec.Store(0)
			server.Tidy()
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
}

func must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}
