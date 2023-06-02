package healthcheck

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/netcheck"
	"tailscale.com/net/portmapper"
	"tailscale.com/prober"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	tslogger "tailscale.com/types/logger"

	"github.com/coder/coder/coderd/util/ptr"
)

type DERPReport struct {
	Healthy bool `json:"healthy"`

	Regions map[int]*DERPRegionReport `json:"regions"`

	Netcheck     *netcheck.Report `json:"netcheck"`
	NetcheckErr  error            `json:"netcheck_err"`
	NetcheckLogs []string         `json:"netcheck_logs"`

	Error error `json:"error"`
}

type DERPRegionReport struct {
	mu      sync.Mutex
	Healthy bool `json:"healthy"`

	Region      *tailcfg.DERPRegion `json:"region"`
	NodeReports []*DERPNodeReport   `json:"node_reports"`
	Error       error               `json:"error"`
}
type DERPNodeReport struct {
	mu            sync.Mutex
	clientCounter int

	Healthy bool              `json:"healthy"`
	Node    *tailcfg.DERPNode `json:"node"`

	ServerInfo          derp.ServerInfoMessage `json:"node_info"`
	CanExchangeMessages bool                   `json:"can_exchange_messages"`
	RoundTripPing       time.Duration          `json:"round_trip_ping"`
	UsesWebsocket       bool                   `json:"uses_websocket"`
	ClientLogs          [][]string             `json:"client_logs"`
	ClientErrs          [][]error              `json:"client_errs"`
	Error               error                  `json:"error"`

	STUN DERPStunReport `json:"stun"`
}

type DERPStunReport struct {
	Enabled bool
	CanSTUN bool
	Error   error
}

type DERPReportOptions struct {
	DERPMap *tailcfg.DERPMap
}

func (r *DERPReport) Run(ctx context.Context, opts *DERPReportOptions) {
	r.Healthy = true
	r.Regions = map[int]*DERPRegionReport{}

	wg := &sync.WaitGroup{}
	mu := sync.Mutex{}

	wg.Add(len(opts.DERPMap.Regions))
	for _, region := range opts.DERPMap.Regions {
		var (
			region       = region
			regionReport = DERPRegionReport{
				Region: region,
			}
		)
		go func() {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					regionReport.Error = xerrors.Errorf("%v", err)
				}
			}()

			regionReport.Run(ctx)

			mu.Lock()
			r.Regions[region.RegionID] = &regionReport
			if !regionReport.Healthy {
				r.Healthy = false
			}
			mu.Unlock()
		}()
	}

	ncLogf := func(format string, args ...interface{}) {
		mu.Lock()
		r.NetcheckLogs = append(r.NetcheckLogs, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	nc := &netcheck.Client{
		PortMapper: portmapper.NewClient(tslogger.WithPrefix(ncLogf, "portmap: "), nil),
		Logf:       tslogger.WithPrefix(ncLogf, "netcheck: "),
	}
	r.Netcheck, r.NetcheckErr = nc.GetReport(ctx, opts.DERPMap)

	wg.Wait()
}

func (r *DERPRegionReport) Run(ctx context.Context) {
	r.Healthy = true
	r.NodeReports = []*DERPNodeReport{}

	wg := &sync.WaitGroup{}

	wg.Add(len(r.Region.Nodes))
	for _, node := range r.Region.Nodes {
		var (
			node       = node
			nodeReport = DERPNodeReport{
				Node:    node,
				Healthy: true,
			}
		)

		go func() {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					nodeReport.Error = xerrors.Errorf("%v", err)
				}
			}()

			nodeReport.Run(ctx)

			r.mu.Lock()
			r.NodeReports = append(r.NodeReports, &nodeReport)
			if !nodeReport.Healthy {
				r.Healthy = false
			}
			r.mu.Unlock()
		}()
	}

	wg.Wait()
}

func (r *DERPNodeReport) derpURL() *url.URL {
	derpURL := &url.URL{
		Scheme: "https",
		Host:   r.Node.HostName,
		Path:   "/derp",
	}
	if r.Node.ForceHTTP {
		derpURL.Scheme = "http"
	}
	if r.Node.HostName == "" {
		derpURL.Host = fmt.Sprintf("%s:%d", r.Node.IPv4, r.Node.DERPPort)
	}

	return derpURL
}

func (r *DERPNodeReport) Run(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	r.ClientLogs = [][]string{}
	r.ClientErrs = [][]error{}

	wg := &sync.WaitGroup{}

	wg.Add(2)
	go func() {
		defer wg.Done()
		r.doExchangeMessage(ctx)
	}()
	go func() {
		defer wg.Done()
		r.doSTUNTest(ctx)
	}()

	wg.Wait()

	// We can't exchange messages with the node,
	if (!r.CanExchangeMessages && !r.Node.STUNOnly) ||
		// A node may use websockets because `Upgrade: DERP` may be blocked on
		// the load balancer. This is unhealthy because websockets are slower
		// than the regular DERP protocol.
		r.UsesWebsocket ||
		// The node was marked as STUN compatible but the STUN test failed.
		r.STUN.Error != nil {
		r.Healthy = false
	}
}

func (r *DERPNodeReport) doExchangeMessage(ctx context.Context) {
	if r.Node.STUNOnly {
		return
	}

	var (
		peerKey  atomic.Pointer[key.NodePublic]
		lastSent atomic.Pointer[time.Time]
	)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wg := &sync.WaitGroup{}

	receive, receiveID, err := r.derpClient(ctx, r.derpURL())
	if err != nil {
		return
	}
	defer receive.Close()

	wg.Add(2)
	go func() {
		defer wg.Done()
		defer receive.Close()

		pkt, err := r.recvData(receive)
		if err != nil {
			r.writeClientErr(receiveID, xerrors.Errorf("recv derp message: %w", err))
			return
		}

		if *peerKey.Load() != pkt.Source {
			r.writeClientErr(receiveID, xerrors.Errorf("received pkt from unknown peer: %s", pkt.Source.ShortString()))
			return
		}

		t := lastSent.Load()

		r.mu.Lock()
		r.CanExchangeMessages = true
		r.RoundTripPing = time.Since(*t)
		r.mu.Unlock()

		cancel()
	}()
	go func() {
		defer wg.Done()
		send, sendID, err := r.derpClient(ctx, r.derpURL())
		if err != nil {
			return
		}
		defer send.Close()

		key := send.SelfPublicKey()
		peerKey.Store(&key)

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		var iter uint8
		for {
			lastSent.Store(ptr.Ref(time.Now()))
			err = send.Send(receive.SelfPublicKey(), []byte{iter})
			if err != nil {
				r.writeClientErr(sendID, xerrors.Errorf("send derp message: %w", err))
				return
			}
			iter++

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	wg.Wait()
}

func (r *DERPNodeReport) doSTUNTest(ctx context.Context) {
	if r.Node.STUNPort == -1 {
		return
	}
	r.mu.Lock()
	r.STUN.Enabled = true
	r.mu.Unlock()

	addr, port, err := r.stunAddr(ctx)
	if err != nil {
		r.STUN.Error = xerrors.Errorf("get stun addr: %w", err)
		return
	}

	// We only create a prober to call ProbeUDP manually.
	p, err := prober.DERP(prober.New(), "", time.Second, time.Second, time.Second)
	if err != nil {
		r.STUN.Error = xerrors.Errorf("create prober: %w", err)
		return
	}

	err = p.ProbeUDP(addr, port)(ctx)
	if err != nil {
		r.STUN.Error = xerrors.Errorf("probe stun: %w", err)
		return
	}

	r.mu.Lock()
	r.STUN.CanSTUN = true
	r.mu.Unlock()
}

func (r *DERPNodeReport) stunAddr(ctx context.Context) (string, int, error) {
	port := r.Node.STUNPort
	if port == 0 {
		port = 3478
	}
	if port < 0 || port > 1<<16-1 {
		return "", 0, xerrors.Errorf("invalid stun port %d", port)
	}

	if r.Node.STUNTestIP != "" {
		ip, err := netip.ParseAddr(r.Node.STUNTestIP)
		if err != nil {
			return "", 0, xerrors.Errorf("invalid stun test ip %q: %w", r.Node.STUNTestIP, err)
		}

		return ip.String(), port, nil
	}

	if r.Node.HostName != "" {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, r.Node.HostName)
		if err != nil {
			return "", 0, xerrors.Errorf("lookup ip addr: %w", err)
		}
		for _, a := range addrs {
			return a.String(), port, nil
		}
	}

	if r.Node.IPv4 != "" {
		ip, err := netip.ParseAddr(r.Node.IPv4)
		if err != nil {
			return "", 0, xerrors.Errorf("invalid ipv4 %q: %w", r.Node.IPv4, err)
		}

		if !ip.Is4() {
			return "", 0, xerrors.Errorf("provided node ipv4 is not v4 %q: %w", r.Node.IPv4, err)
		}

		return ip.String(), port, nil
	}
	if r.Node.IPv6 != "" {
		ip, err := netip.ParseAddr(r.Node.IPv6)
		if err != nil {
			return "", 0, xerrors.Errorf("invalid ipv6 %q: %w", r.Node.IPv6, err)
		}

		if !ip.Is6() {
			return "", 0, xerrors.Errorf("provided node ipv6 is not v6 %q: %w", r.Node.IPv6, err)
		}

		return ip.String(), port, nil
	}

	return "", 0, xerrors.New("no stun ips provided")
}

func (r *DERPNodeReport) writeClientErr(clientID int, err error) {
	r.mu.Lock()
	r.ClientErrs[clientID] = append(r.ClientErrs[clientID], err)
	r.mu.Unlock()
}

func (r *DERPNodeReport) derpClient(ctx context.Context, derpURL *url.URL) (*derphttp.Client, int, error) {
	r.mu.Lock()
	id := r.clientCounter
	r.clientCounter++
	r.ClientLogs = append(r.ClientLogs, []string{})
	r.ClientErrs = append(r.ClientErrs, []error{})
	r.mu.Unlock()

	client, err := derphttp.NewClient(key.NewNode(), derpURL.String(), func(format string, args ...any) {
		r.mu.Lock()
		defer r.mu.Unlock()

		msg := fmt.Sprintf(format, args...)
		if strings.Contains(msg, "We'll use WebSockets on the next connection attempt") {
			r.UsesWebsocket = true
		}
		r.ClientLogs[id] = append(r.ClientLogs[id], msg)
	})
	if err != nil {
		err := xerrors.Errorf("create derp client: %w", err)
		r.writeClientErr(id, err)
		return nil, id, err
	}

	go func() {
		<-ctx.Done()
		_ = client.Close()
	}()

	i := 0
	for ; i < 5; i++ {
		err = client.Connect(ctx)
		if err != nil {
			r.writeClientErr(id, xerrors.Errorf("connect to derp: %w", err))
			continue
		}
		break
	}
	if i == 5 {
		err := xerrors.Errorf("couldn't connect after 5 tries, last error: %w", err)
		r.writeClientErr(id, xerrors.Errorf("couldn't connect after 5 tries, last error: %w", err))
		return nil, id, err
	}

	return client, id, nil
}

func (r *DERPNodeReport) recvData(client *derphttp.Client) (derp.ReceivedPacket, error) {
	for {
		msg, err := client.Recv()
		if err != nil {
			return derp.ReceivedPacket{}, err
		}

		switch msg := msg.(type) {
		case derp.ReceivedPacket:
			return msg, nil
		case derp.ServerInfoMessage:
			r.mu.Lock()
			r.ServerInfo = msg
			r.mu.Unlock()
		default:
			// Drop all others!
		}
	}
}
