package tailnet
import (
	"fmt"
	"errors"
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"sync"
	"time"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"tailscale.com/tailcfg"
	"tailscale.com/types/netmap"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/tailnet/proto"
)
const (
	TelemetryApplicationSSH       string = "ssh"
	TelemetryApplicationSpeedtest string = "speedtest"
	TelemetryApplicationVSCode    string = "vscode"
)
// Responsible for storing and anonymizing networking telemetry state.
type TelemetryStore struct {
	mu       sync.Mutex
	hashSalt string
	// A cache to avoid hashing the same IP or hostname multiple times.
	hashCache map[string]string
	cleanDerpMap  *tailcfg.DERPMap
	cleanNetCheck *proto.Netcheck
	nodeIDSelf    uint64
	homeDerp      int32
	application   string
	// nil if not connected
	connectedIP *netip.Addr
	// 0 if not connected
	nodeIDRemote uint64
	p2p          bool
	p2pSetupTime time.Duration
	lastDerpTime time.Time
}
func newTelemetryStore() (*TelemetryStore, error) {
	hashSalt, err := cryptorand.String(16)
	if err != nil {
		return nil, err
	}
	return &TelemetryStore{
		hashSalt:  hashSalt,
		hashCache: make(map[string]string),
	}, nil
}
// newEvent returns the current telemetry state as an event
func (b *TelemetryStore) newEvent() *proto.TelemetryEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := &proto.TelemetryEvent{
		Time:           timestamppb.Now(),
		ClientVersion:  buildinfo.Version(),
		DerpMap:        DERPMapToProto(b.cleanDerpMap),
		LatestNetcheck: b.cleanNetCheck,
		NodeIdSelf:     b.nodeIDSelf,
		NodeIdRemote:   b.nodeIDRemote,
		HomeDerp:       b.homeDerp,
		Application:    b.application,
	}
	if b.p2pSetupTime > 0 {
		out.P2PSetup = durationpb.New(b.p2pSetupTime)
	}
	return out
}
func (b *TelemetryStore) markConnected(ip *netip.Addr, application string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastDerpTime = time.Now()
	b.connectedIP = ip
	b.application = application
}
func (b *TelemetryStore) pingPeer(conn *Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.connectedIP == nil {
		return
	}
	ip := *b.connectedIP
	go func() {
		_, _, _, _ = conn.Ping(conn.watchCtx, ip)
	}()
}
func (b *TelemetryStore) changedConntype(addr string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.p2p && addr != "" {
		return false
	} else if !b.p2p && addr != "" {
		b.p2p = true
		b.p2pSetupTime = time.Since(b.lastDerpTime)
		return true
	} else if b.p2p && addr == "" {
		b.p2p = false
		b.lastDerpTime = time.Now()
		b.p2pSetupTime = 0
		return true
	}
	return false
}
func (b *TelemetryStore) updateRemoteNodeIDLocked(nm *netmap.NetworkMap) {
	if b.connectedIP == nil {
		return
	}
	ip := *b.connectedIP
	for _, p := range nm.Peers {
		for _, a := range p.Addresses {
			if a.Addr() == ip && a.IsSingleIP() {
				b.nodeIDRemote = uint64(p.ID)
			}
		}
	}
}
func (b *TelemetryStore) updateNetworkMap(nm *netmap.NetworkMap) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if nm == nil {
		return
	}
	b.updateDerpMapLocked(nm.DERPMap)
	b.updateRemoteNodeIDLocked(nm)
	b.updateByNodeLocked(nm.SelfNode)
}
// Given a DERPMap, anonymise all IPs and hostnames.
// Keep track of seen hostnames/cert names to anonymize them from future logs.
// b.mu must NOT be held.
func (b *TelemetryStore) updateDerpMap(cur *tailcfg.DERPMap) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.updateDerpMapLocked(cur)
}
func (b *TelemetryStore) updateDerpMapLocked(cur *tailcfg.DERPMap) {
	if cur == nil {
		return
	}
	cleanMap := cur.Clone()
	for _, r := range cleanMap.Regions {
		for _, n := range r.Nodes {
			ipv4, _, _ := b.processIPLocked(n.IPv4)
			n.IPv4 = ipv4
			ipv6, _, _ := b.processIPLocked(n.IPv6)
			n.IPv6 = ipv6
			stunIP, _, _ := b.processIPLocked(n.STUNTestIP)
			n.STUNTestIP = stunIP
			hn := b.hashAddrorHostname(n.HostName)
			n.HostName = hn
			cn := b.hashAddrorHostname(n.CertName)
			n.CertName = cn
		}
	}
	b.cleanDerpMap = cleanMap
}
// Update the telemetry store with the current self node state.
// Returns true if the home DERP has changed.
func (b *TelemetryStore) updateByNodeLocked(n *tailcfg.Node) bool {
	if n == nil {
		return false
	}
	b.nodeIDSelf = uint64(n.ID)
	derpIP, err := netip.ParseAddrPort(n.DERP)
	if err != nil {
		return false
	}
	newHome := int32(derpIP.Port())
	if b.homeDerp != newHome {
		b.homeDerp = newHome
		return true
	}
	return false
}
// Store an anonymized proto.Netcheck given a tailscale NetInfo.
func (b *TelemetryStore) setNetInfo(ni *tailcfg.NetInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cleanNetCheck = &proto.Netcheck{
		UDP:                   ni.UDP,
		IPv6:                  ni.IPv6,
		IPv4:                  ni.IPv4,
		IPv6CanSend:           ni.IPv6CanSend,
		IPv4CanSend:           ni.IPv4CanSend,
		ICMPv4:                ni.ICMPv4,
		OSHasIPv6:             wrapperspb.Bool(ni.OSHasIPv6.EqualBool(true)),
		MappingVariesByDestIP: wrapperspb.Bool(ni.MappingVariesByDestIP.EqualBool(true)),
		HairPinning:           wrapperspb.Bool(ni.HairPinning.EqualBool(true)),
		UPnP:                  wrapperspb.Bool(ni.UPnP.EqualBool(true)),
		PMP:                   wrapperspb.Bool(ni.PMP.EqualBool(true)),
		PCP:                   wrapperspb.Bool(ni.PCP.EqualBool(true)),
		PreferredDERP:         int64(ni.PreferredDERP),
		RegionV4Latency:       make(map[int64]*durationpb.Duration),
		RegionV6Latency:       make(map[int64]*durationpb.Duration),
	}
	v4hash, v4fields, err := b.processIPLocked(ni.GlobalV4)
	if err == nil {
		b.cleanNetCheck.GlobalV4 = &proto.Netcheck_NetcheckIP{
			Hash:   v4hash,
			Fields: v4fields,
		}
	}
	v6hash, v6fields, err := b.processIPLocked(ni.GlobalV6)
	if err == nil {
		b.cleanNetCheck.GlobalV6 = &proto.Netcheck_NetcheckIP{
			Hash:   v6hash,
			Fields: v6fields,
		}
	}
	for rid, seconds := range ni.DERPLatencyV4 {
		b.cleanNetCheck.RegionV4Latency[int64(rid)] = durationpb.New(time.Duration(seconds * float64(time.Second)))
	}
	for rid, seconds := range ni.DERPLatencyV6 {
		b.cleanNetCheck.RegionV6Latency[int64(rid)] = durationpb.New(time.Duration(seconds * float64(time.Second)))
	}
}
func (b *TelemetryStore) toEndpoint(ipport string) *proto.TelemetryEvent_P2PEndpoint {
	b.mu.Lock()
	defer b.mu.Unlock()
	addrport, err := netip.ParseAddrPort(ipport)
	if err != nil {
		return nil
	}
	addr := addrport.Addr()
	fields := addrToFields(addr)
	hashStr := b.hashAddrorHostname(addr.String())
	return &proto.TelemetryEvent_P2PEndpoint{
		Hash:   hashStr,
		Port:   int32(addrport.Port()),
		Fields: fields,
	}
}
// processIPLocked will look up the IP in the cache, or hash and salt it and add
// to the cache. It will also add it to hashedIPs.
//
// b.mu must be held.
func (b *TelemetryStore) processIPLocked(ip string) (string, *proto.IPFields, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse IP %q: %w", ip, err)
	}
	fields := addrToFields(addr)
	hashStr := b.hashAddrorHostname(ip)
	return hashStr, fields, nil
}
func (b *TelemetryStore) hashAddrorHostname(addr string) string {
	if hashStr, ok := b.hashCache[addr]; ok {
		return hashStr
	}
	hash := sha256.Sum256([]byte(b.hashSalt + addr))
	hashStr := hex.EncodeToString(hash[:])
	b.hashCache[addr] = hashStr
	return hashStr
}
func addrToFields(addr netip.Addr) *proto.IPFields {
	version := int32(4)
	if addr.Is6() {
		version = 6
	}
	class := proto.IPFields_PUBLIC
	switch {
	case addr.IsLoopback():
		class = proto.IPFields_LOOPBACK
	case addr.IsLinkLocalUnicast():
		class = proto.IPFields_LINK_LOCAL
	case addr.IsLinkLocalMulticast():
		class = proto.IPFields_LINK_LOCAL
	case addr.IsPrivate():
		class = proto.IPFields_PRIVATE
	}
	return &proto.IPFields{
		Version: version,
		Class:   class,
	}
}
