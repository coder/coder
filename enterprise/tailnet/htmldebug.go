package tailnet

import (
	"context"
	"database/sql"
	"html/template"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	gProto "google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/tailnet/proto"
)

type HTMLDebug struct {
	Coordinators []*HTMLCoordinator
	Peers        []*HTMLPeer
	Tunnels      []*HTMLTunnel
}

type HTMLPeer struct {
	ID            uuid.UUID
	CoordinatorID uuid.UUID
	LastWriteAge  time.Duration
	Node          string
	Status        database.TailnetStatus
}

type HTMLCoordinator struct {
	ID           uuid.UUID
	HeartbeatAge time.Duration
}

type HTMLTunnel struct {
	CoordinatorID uuid.UUID
	SrcID         uuid.UUID
	DstID         uuid.UUID
	LastWriteAge  time.Duration
}

func (c *pgCoord) ServeHTTPDebug(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	debug, err := getDebug(ctx, c.store)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	err = debugTempl.Execute(w, debug)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
}

func getDebug(ctx context.Context, store database.Store) (HTMLDebug, error) {
	out := HTMLDebug{}
	coords, err := store.GetAllTailnetCoordinators(ctx)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return HTMLDebug{}, xerrors.Errorf("failed to query coordinators: %w", err)
	}
	peers, err := store.GetAllTailnetPeers(ctx)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return HTMLDebug{}, xerrors.Errorf("failed to query peers: %w", err)
	}
	tunnels, err := store.GetAllTailnetTunnels(ctx)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return HTMLDebug{}, xerrors.Errorf("failed to query tunnels: %w", err)
	}
	now := time.Now() // call this once so all our ages are on the same timebase
	for _, coord := range coords {
		out.Coordinators = append(out.Coordinators, coordToHTML(coord, now))
	}
	for _, peer := range peers {
		ph, err := peerToHTML(peer, now)
		if err != nil {
			return HTMLDebug{}, err
		}
		out.Peers = append(out.Peers, ph)
	}
	for _, tunnel := range tunnels {
		out.Tunnels = append(out.Tunnels, tunnelToHTML(tunnel, now))
	}
	return out, nil
}

func coordToHTML(d database.TailnetCoordinator, now time.Time) *HTMLCoordinator {
	return &HTMLCoordinator{
		ID:           d.ID,
		HeartbeatAge: now.Sub(d.HeartbeatAt),
	}
}

func peerToHTML(d database.TailnetPeer, now time.Time) (*HTMLPeer, error) {
	node := &proto.Node{}
	err := gProto.Unmarshal(d.Node, node)
	if err != nil {
		return nil, xerrors.Errorf("unmarshal node: %w", err)
	}
	return &HTMLPeer{
		ID:            d.ID,
		CoordinatorID: d.CoordinatorID,
		LastWriteAge:  now.Sub(d.UpdatedAt),
		Status:        d.Status,
		Node:          node.String(),
	}, nil
}

func tunnelToHTML(d database.TailnetTunnel, now time.Time) *HTMLTunnel {
	return &HTMLTunnel{
		CoordinatorID: d.CoordinatorID,
		SrcID:         d.SrcID,
		DstID:         d.DstID,
		LastWriteAge:  now.Sub(d.UpdatedAt),
	}
}

var coordinatorDebugTmpl = `
<!DOCTYPE html>
<html>
	<head>
		<meta charset="UTF-8">
		<style>
th, td {
  padding-top: 6px;
  padding-bottom: 6px;
  padding-left: 10px;
  padding-right: 10px;
  text-align: left;
}
tr {
  border-bottom: 1px solid #ddd;
}
		</style>
	</head>
	<body>
		<h2 id=coordinators><a href=#coordinators>#</a> coordinators: total {{ len .Coordinators }}</h2>
		<table>
			<tr style="margin-top:4px">
				<th>ID</th>
				<th>Heartbeat Age</th>
			</tr>
		{{- range .Coordinators}}
			<tr style="margin-top:4px">
				<td>{{ .ID }}</td>
				<td>{{ .HeartbeatAge }} ago</td>
			</tr>
		{{- end }}
		</table>

		<h2 id=peers> <a href=#peers>#</a> peers: total {{ len .Peers }} </h2>
		<table>
			<tr style="margin-top:4px">
				<th>ID</th>
				<th>CoordinatorID</th>
				<th>Status</th>
				<th>Last Write Age</th>
				<th>Node</th>
			</tr>
		{{- range .Peers }}
			<tr style="margin-top:4px">
				<td>{{ .ID }}</td>
				<td>{{ .CoordinatorID }}</td>
				<td>{{ .Status }}</td>
				<td>{{ .LastWriteAge }} ago</td>
				<td style="white-space: pre;"><code>{{ .Node }}</code></td>
			</tr>
		{{- end }}
		</table>

		<h2 id=tunnels><a href=#tunnels>#</a> tunnels: total {{ len .Tunnels }}</h2>
		<table>
			<tr style="margin-top:4px">
				<th>SrcID</th>
				<th>DstID</th>
				<th>CoordinatorID</th>
				<th>Last Write Age</th>
			</tr>
		{{- range .Tunnels }}
			<tr style="margin-top:4px">
				<td>{{ .SrcID }}</td>
				<td>{{ .DstID }}</td>
				<td>{{ .CoordinatorID }}</td>
				<td>{{ .LastWriteAge }} ago</td>
			</tr>
		{{- end }}
		</table>
	</body>
</html>
`

var debugTempl = template.Must(template.New("coordinator_debug").Parse(coordinatorDebugTmpl))
