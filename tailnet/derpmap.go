package tailnet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/cryptorand"
)

func STUNNodes(regionID int, stunAddrs []string) ([]*tailcfg.DERPNode, error) {
	nodes := []*tailcfg.DERPNode{}
	for index, stunAddr := range stunAddrs {
		if stunAddr == "disable" {
			return []*tailcfg.DERPNode{}, nil
		}

		host, rawPort, err := net.SplitHostPort(stunAddr)
		if err != nil {
			return nil, xerrors.Errorf("split host port for %q: %w", stunAddr, err)
		}
		port, err := strconv.Atoi(rawPort)
		if err != nil {
			return nil, xerrors.Errorf("parse port for %q: %w", stunAddr, err)
		}
		nodes = append([]*tailcfg.DERPNode{{
			Name:     fmt.Sprintf("%dstun%d", regionID, index),
			RegionID: regionID,
			HostName: host,
			STUNOnly: true,
			STUNPort: port,
		}}, nodes...)
	}

	return nodes, nil
}

// NewDERPMap constructs a DERPMap from a set of STUN addresses and optionally a remote
// URL to fetch a mapping from e.g. https://controlplane.tailscale.com/derpmap/default.
//
//nolint:revive
func NewDERPMap(ctx context.Context, region *tailcfg.DERPRegion, stunAddrs []string, remoteURL, localPath string, disableSTUN bool) (*tailcfg.DERPMap, error) {
	if remoteURL != "" && localPath != "" {
		return nil, xerrors.New("a remote URL or local path must be specified, not both")
	}
	if disableSTUN {
		stunAddrs = nil
	}

	if region != nil {
		stunNodes, err := STUNNodes(region.RegionID, stunAddrs)
		if err != nil {
			return nil, xerrors.Errorf("construct stun nodes: %w", err)
		}

		region.Nodes = append(stunNodes, region.Nodes...)
	}

	derpMap := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{},
	}
	if remoteURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
		if err != nil {
			return nil, xerrors.Errorf("create request: %w", err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, xerrors.Errorf("get derpmap: %w", err)
		}
		defer res.Body.Close()
		err = json.NewDecoder(res.Body).Decode(&derpMap)
		if err != nil {
			return nil, xerrors.Errorf("fetch derpmap: %w", err)
		}
	}
	if localPath != "" {
		content, err := os.ReadFile(localPath)
		if err != nil {
			return nil, xerrors.Errorf("read derpmap from %q: %w", localPath, err)
		}
		err = json.Unmarshal(content, &derpMap)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal derpmap: %w", err)
		}
	}
	if region != nil {
		_, conflicts := derpMap.Regions[region.RegionID]
		if conflicts {
			return nil, xerrors.Errorf("the default region ID conflicts with a remote region from %q", remoteURL)
		}
		derpMap.Regions[region.RegionID] = region
	}
	// Remove all STUNPorts from DERPy nodes, and fully remove all STUNOnly
	// nodes.
	if disableSTUN {
		for _, region := range derpMap.Regions {
			newNodes := make([]*tailcfg.DERPNode, 0, len(region.Nodes))
			for _, node := range region.Nodes {
				node.STUNPort = -1
				if !node.STUNOnly {
					newNodes = append(newNodes, node)
				}
			}
			region.Nodes = newNodes
		}
	}

	return derpMap, nil
}

type DERPMapProvider interface {
	io.Closer

	// Get returns the current value of the DERP map.
	Get() *tailcfg.DERPMap
	// UpdateNow forces an immediate update of the DERP map if it is generated
	// dynamically.
	UpdateNow(ctx context.Context) (*tailcfg.DERPMap, error)
	// Subscribe adds the given function to the list of subscribers that will be
	// notified when the DERP map changes. The function is called immediately with
	// the current value.
	//
	// The subscription function MUST NOT BLOCK.
	Subscribe(fn func(*tailcfg.DERPMap)) (func(), error)
}

// DERPMapProviderStatic is a DERPMapProvider that always returns the same DERP
// map.
type DERPMapProviderStatic struct {
	derpMap *tailcfg.DERPMap
}

var _ DERPMapProvider = &DERPMapProviderStatic{}

func NewDERPMapProviderStatic(derpMap *tailcfg.DERPMap) DERPMapProvider {
	return &DERPMapProviderStatic{
		derpMap: derpMap,
	}
}

// Close implements io.Closer. This is a no-op.
func (p *DERPMapProviderStatic) Close() error {
	return nil
}

// Get implements DERPMapProvider.
func (p *DERPMapProviderStatic) Get() *tailcfg.DERPMap {
	return p.derpMap.Clone()
}

// UpdateNow implements DERPMapProvider. This is a no-op.
func (p *DERPMapProviderStatic) UpdateNow(_ context.Context) (*tailcfg.DERPMap, error) {
	return p.derpMap.Clone(), nil
}

// Subscribe implements DERPMapProvider.
func (p *DERPMapProviderStatic) Subscribe(fn func(*tailcfg.DERPMap)) (func(), error) {
	fn(p.derpMap.Clone())
	return func() {}, nil
}

// DERPMapGetterFn is a function that returns a generated DERP map.
type DERPMapGetterFn func(ctx context.Context) (*tailcfg.DERPMap, error)

// DERPMapProviderGetterFn is a DERPMapProvider that calls a function to get the
// DERP map. The function is called periodically (or when UpdateNow is called)
type DERPMapProviderGetterFn struct {
	ctx             context.Context
	cancel          context.CancelFunc
	log             slog.Logger
	done            chan struct{}
	getterFn        DERPMapGetterFn
	updateFrequency time.Duration

	mut           sync.RWMutex
	last          *tailcfg.DERPMap
	subscriptions map[string]func(*tailcfg.DERPMap)
}

var _ DERPMapProvider = &DERPMapProviderGetterFn{}

func NewDERPMapProviderGetterFn(ctx context.Context, log slog.Logger, getterFn DERPMapGetterFn) (DERPMapProvider, error) {
	ctx, cancel := context.WithCancel(ctx)
	p := &DERPMapProviderGetterFn{
		ctx:      ctx,
		cancel:   cancel,
		log:      log,
		done:     make(chan struct{}),
		getterFn: getterFn,
		// TODO: configurable
		updateFrequency: 5 * time.Second,

		subscriptions: map[string]func(*tailcfg.DERPMap){},
	}

	_, err := p.UpdateNow(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	go p.updateLoop(ctx)
	return p, nil
}

// Close implements io.Closer.
func (p *DERPMapProviderGetterFn) Close() error {
	p.cancel()
	<-p.done
	return nil
}

// UpdateNow implements DERPMapProvider.
func (p *DERPMapProviderGetterFn) UpdateNow(ctx context.Context) (*tailcfg.DERPMap, error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	newMap, err := p.getterFn(ctx)
	if err != nil {
		p.log.Warn(ctx, "failed to update DERP map", slog.Error(err))
		return nil, err
	}
	if !CompareDERPMaps(p.last, newMap) {
		p.last = newMap
		p.notifyAll()
	}

	return p.last.Clone(), nil
}

func (p *DERPMapProviderGetterFn) updateLoop(ctx context.Context) {
	defer close(p.done)

	ticker := time.NewTicker(p.updateFrequency)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// This function logs its own errors.
			_, _ = p.UpdateNow(ctx)
		}
		ticker.Reset(p.updateFrequency)
	}
}

// Get implements DERPMapProvider.
func (p *DERPMapProviderGetterFn) Get() *tailcfg.DERPMap {
	p.mut.RLock()
	defer p.mut.RUnlock()
	return p.last
}

// Subscribe implements DERPMapProvider.
func (p *DERPMapProviderGetterFn) Subscribe(fn func(*tailcfg.DERPMap)) (func(), error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	key, err := cryptorand.String(16)
	if err != nil {
		return nil, xerrors.Errorf("generate random subscription key: %w", err)
	}
	if _, ok := p.subscriptions[key]; ok {
		return nil, xerrors.Errorf("generated subscription key %q already exists, please try again", key)
	}

	p.subscriptions[key] = fn
	fn(p.last.Clone())
	return func() {
		p.mut.Lock()
		defer p.mut.Unlock()
		delete(p.subscriptions, key)
	}, nil
}

func (p *DERPMapProviderGetterFn) notifyAll() {
	for _, fn := range p.subscriptions {
		fn(p.last.Clone())
	}
}

// CompareDERPMaps returns true if the given DERPMaps are equivalent. Ordering
// of slices is ignored.
//
// If the first map is nil, the second map must also be nil for them to be
// considered equivalent. If the second map is nil, the first map can be any
// value and the function will return true.
func CompareDERPMaps(a *tailcfg.DERPMap, b *tailcfg.DERPMap) bool {
	if a == nil {
		return b == nil
	}
	if b == nil {
		return true
	}
	if len(a.Regions) != len(b.Regions) {
		return false
	}
	if a.OmitDefaultRegions != b.OmitDefaultRegions {
		return false
	}

	for id, region := range a.Regions {
		other, ok := b.Regions[id]
		if !ok {
			return false
		}
		if !compareDERPRegions(region, other) {
			return false
		}
	}
	return true
}

func compareDERPRegions(a *tailcfg.DERPRegion, b *tailcfg.DERPRegion) bool {
	if a == nil || b == nil {
		return false
	}
	if a.EmbeddedRelay != b.EmbeddedRelay {
		return false
	}
	if a.RegionID != b.RegionID {
		return false
	}
	if a.RegionCode != b.RegionCode {
		return false
	}
	if a.RegionName != b.RegionName {
		return false
	}
	if a.Avoid != b.Avoid {
		return false
	}
	if len(a.Nodes) != len(b.Nodes) {
		return false
	}

	// Convert both slices to maps so ordering can be ignored easier.
	aNodes := map[string]*tailcfg.DERPNode{}
	for _, node := range a.Nodes {
		aNodes[node.Name] = node
	}
	bNodes := map[string]*tailcfg.DERPNode{}
	for _, node := range b.Nodes {
		bNodes[node.Name] = node
	}

	for name, aNode := range aNodes {
		bNode, ok := bNodes[name]
		if !ok {
			return false
		}

		if aNode.Name != bNode.Name {
			return false
		}
		if aNode.RegionID != bNode.RegionID {
			return false
		}
		if aNode.HostName != bNode.HostName {
			return false
		}
		if aNode.CertName != bNode.CertName {
			return false
		}
		if aNode.IPv4 != bNode.IPv4 {
			return false
		}
		if aNode.IPv6 != bNode.IPv6 {
			return false
		}
		if aNode.STUNPort != bNode.STUNPort {
			return false
		}
		if aNode.STUNOnly != bNode.STUNOnly {
			return false
		}
		if aNode.DERPPort != bNode.DERPPort {
			return false
		}
		if aNode.InsecureForTests != bNode.InsecureForTests {
			return false
		}
		if aNode.ForceHTTP != bNode.ForceHTTP {
			return false
		}
		if aNode.STUNTestIP != bNode.STUNTestIP {
			return false
		}
	}

	return true
}
