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
	"strings"
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

type CacheItem struct {
	*Peer
	ExpireAt time.Time
}

func ClearCache[T comparable](cache map[T]*CacheItem) {
	now := time.Now()
	for k, c := range cache {
		if c.ExpireAt.Compare(now) >= 0 {
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
	peer, ok := pm.Peers[addr.String()]
	if !ok {
		pm.Peers[key] = &CacheItem{&Peer{RInfo: addr}, time.Now().Add(30 * time.Second)}
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
		PeerManager:  &PeerManager{},
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
		break
	case Ipv4:
		server.OnIpv4(peer, payload)
		break
	case Ping:
		slog.Error("encrypted ping?")
		break
	case Ipv4Frag:
		server.OnIpv4Frag(peer, payload)
		break
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
				slog.Error("%v | user: %v", err, username)
				server.SendInfo(peer, err.Error())
			}
		}
	} else {
		if peer.Challenge == nil {
			randBytes := make([]byte, 65)
			rand.Read(randBytes)
			randBytes[0] = 0
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

func (server *SLPServer) Run() context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	server.SendChan = make(chan Packet, 2)

	go func() {
		conn, err := net.ListenUDP("udp6", must(net.ResolveUDPAddr("udp", ":"+fmt.Sprint(server.Port))))
		if err != nil {
			log.Fatalf("[FATAL] Couldn't listen on %v: %v\f", conn.LocalAddr().String(), err)
		}
		defer conn.Close()

		buffer := make([]byte, 2048)

		slog.Info("Server listening on %v", conn.LocalAddr().String())

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
				_, err := conn.WriteToUDP(packet.Msg, packet.Addr)
				if err != nil {
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
			str := fmt.Sprintf("Clients: %v | Upload: %vKB/s | Dowload: %vKB/s", server.GetClientSize(), server.UploadLastSec.Load(), server.DownloadLastSec.Load())
			fmt.Printf(str)
			fmt.Printf(strings.Repeat("\b", len(str)))
			server.Tidy()
			select {
			case <-ctx.Done():
				return
			default:
			}
			time.Sleep(time.Second)
		}
	}()

	return cancel
}

func must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}
