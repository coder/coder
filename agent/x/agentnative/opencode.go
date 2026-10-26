package agentnative

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

// OpenCode observes a dedicated native V2 server. The server and its database
// outlive this observer; only Shutdown may stop a verified owned process.
type OpenCode struct {
	mu         sync.Mutex
	host       Host
	clock      quartz.Clock
	server     *openCodeServer
	client     *openCodeRPC
	sessions   map[string]*openCodeObservation
	ctx        context.Context
	cancel     context.CancelFunc
	wake       chan struct{}
	generation uint64
	started    bool
	wg         sync.WaitGroup

	// Ingress must not wait for the state mutex held during HTTP snapshots.
	eventMu       sync.Mutex
	eventSessions map[string]bool
	pending       openCodeWake
}

type openCodeObservation struct {
	session Session
	ready   bool
	fault   bool
}

type openCodeWake struct {
	generation uint64
	dirty      map[string]bool
	critical   []openCodeNativeEvent
	lost       bool
}

var _ Backend = (*OpenCode)(nil)

// NewOpenCode is passive. Create and Resume may start a missing dedicated server;
// Attach never does.
func NewOpenCode(options BackendOptions) (*OpenCode, error) {
	if options.Host == nil || options.StateDir == "" || !filepath.IsAbs(options.StateDir) {
		return nil, xerrors.New("OpenCode requires a host and absolute private state directory")
	}
	clock := options.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &OpenCode{host: options.Host, clock: clock, server: newOpenCodeServer(options, clock), sessions: map[string]*openCodeObservation{}, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), eventSessions: map[string]bool{}}, nil
}

func (o *OpenCode) connect(ctx context.Context) error {
	if o.ctx.Err() != nil {
		return xerrors.New("OpenCode observer is closed")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if o.client != nil && o.client.ctx.Err() == nil {
		return nil
	}
	if o.client != nil {
		o.client.close()
		o.client = nil
	}
	descriptor, err := o.server.ensure(ctx)
	if err != nil {
		return err
	}
	o.generation++
	generation := o.generation
	o.eventMu.Lock()
	o.pending = openCodeWake{generation: generation}
	o.eventMu.Unlock()
	lifetime, cancel := context.WithCancel(o.ctx)
	client, err := newOpenCodeRPC(ctx, lifetime, descriptor, o.clock, o.server.http, func(event openCodeNativeEvent) {
		if !o.enqueue(generation, event) {
			cancel()
		}
	})
	if err != nil {
		cancel()
		return err
	}
	o.client = client
	if !o.started {
		o.started = true
		o.wg.Add(1)
		go o.observe()
	}
	return nil
}

type openCodeSessionInfo struct {
	ID       string `json:"id"`
	Location struct {
		Directory string `json:"directory"`
	} `json:"location"`
	Time struct {
		Created  *int64          `json:"created"`
		Archived json.RawMessage `json:"archived"`
	} `json:"time"`
}

func (info openCodeSessionInfo) valid(session Session) bool {
	return openCodeSessionID.MatchString(info.ID) && info.ID == session.NativeID &&
		info.Location.Directory == session.Cwd && filepath.IsAbs(info.Location.Directory) && len(info.Location.Directory) <= 4096 &&
		info.Time.Created != nil && len(info.Time.Archived) == 0
}

func openCodeValidSession(session Session) bool {
	return session.Harness == "opencode" && openCodeSessionID.MatchString(session.NativeID) && session.ID == "opencode:"+session.NativeID && filepath.IsAbs(session.Cwd) && len(session.Cwd) <= 4096
}

// Create allocates identity only. The manager registers it before calling Resume.
func (o *OpenCode) Create(ctx context.Context, cwd string) (Session, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !filepath.IsAbs(cwd) || len(cwd) > 4096 {
		return Session{}, xerrors.New("OpenCode cwd must be absolute")
	}
	if err := o.connect(ctx); err != nil {
		return Session{}, err
	}
	var result struct {
		Data openCodeSessionInfo `json:"data"`
	}
	if err := o.client.request(ctx, "POST", "/api/session", map[string]any{"location": map[string]string{"directory": cwd}}, &result); err != nil {
		return Session{}, err
	}
	session := Session{ID: "opencode:" + result.Data.ID, NativeID: result.Data.ID, Harness: "opencode", Cwd: cwd, Mode: "human"}
	if !result.Data.valid(session) {
		return Session{}, errOpenCodeProtocol
	}
	session.StartedAt = *result.Data.Time.Created
	return session, nil
}

// Resume starts the dedicated server when necessary, then only reads the saved
// native session. It never creates a replacement session or resubmits input.
func (o *OpenCode) Resume(ctx context.Context, session Session) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !openCodeValidSession(session) {
		return xerrors.New("invalid OpenCode session identity")
	}
	if _, _, err := o.saved(session); err != nil {
		return err
	}
	if err := o.connect(ctx); err != nil {
		return err
	}
	observation := &openCodeObservation{session: session}
	o.sessions[session.ID] = observation
	o.eventMu.Lock()
	o.eventSessions[session.NativeID] = true
	o.eventMu.Unlock()
	if _, _, err := o.snapshot(ctx, observation, openCodeResumeSnapshot); err != nil {
		o.fault(observation)
		return err
	}
	observation.ready = true
	return nil
}

func (o *OpenCode) current(session Session) (*openCodeObservation, error) {
	if !openCodeValidSession(session) {
		return nil, xerrors.New("invalid OpenCode session identity")
	}
	observation := o.sessions[session.ID]
	if o.ctx.Err() != nil || o.client == nil || o.client.ctx.Err() != nil || observation == nil || !observation.ready || observation.fault || observation.session.NativeID != session.NativeID || observation.session.Cwd != session.Cwd {
		return nil, xerrors.New("OpenCode requires read-only resume before this operation")
	}
	return observation, nil
}

func (o *OpenCode) saved(session Session) (Session, *openCodeReceipts, error) {
	saved, err := o.host.Session(session.ID)
	if err != nil || saved.NativeID != session.NativeID || saved.Cwd != session.Cwd || saved.Harness != "opencode" {
		return Session{}, nil, xerrors.New("OpenCode saved identity could not be verified")
	}
	receipts, err := readOpenCodeReceipts(saved.State)
	return saved, receipts, err
}

func (o *OpenCode) publish(id string, events []Event, receipts *openCodeReceipts) error {
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil || len(encoded)+32 > 128<<10 {
			return errOpenCodeProtocol
		}
	}
	state, err := json.Marshal(receipts)
	if err != nil || len(state) > 256<<10 {
		return errOpenCodeProtocol
	}
	if len(events) == 0 {
		current, err := o.host.Session(id)
		if err != nil {
			return xerrors.New("OpenCode saved session unavailable")
		}
		if bytes.Equal(current.State, state) {
			return nil
		}
	}
	if err := o.host.Publish(id, events, state); err != nil {
		return xerrors.New("OpenCode observation persistence failed; no work was replayed")
	}
	return nil
}

func (o *OpenCode) fault(observation *openCodeObservation) {
	if observation.fault {
		return
	}
	observation.ready = false
	observation.fault = true
	_ = o.host.Publish(observation.session.ID, []Event{{Type: "overflow", Data: map[string]any{"reason": "OpenCode observation lost or history could not be reconciled; resume to inspect native state. No work was replayed."}}}, nil)
}

// Prompt records native ownership before its single, never-retried mutation.
func (o *OpenCode) Prompt(ctx context.Context, session Session, prompt Prompt) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !openCodeValidatePrompt(prompt) {
		return xerrors.New("invalid OpenCode prompt")
	}
	observation, err := o.current(session)
	if err != nil {
		return err
	}
	_, receipts, err := o.saved(session)
	if err != nil {
		return err
	}
	if old := receipts.Requests[prompt.ID]; old != nil {
		if old.Hash != openCodeHash(prompt.Text) {
			return xerrors.New("OpenCode request ID already used for different text")
		}
		return nil
	}
	receipts, busy, err := o.snapshot(ctx, observation, openCodeLiveSnapshot)
	if err != nil {
		o.fault(observation)
		return err
	}
	saved, err := o.host.Session(session.ID)
	if err != nil {
		return xerrors.New("OpenCode saved session unavailable")
	}
	if saved.Mode != "automation" || saved.Archived || saved.ResumePending || saved.Fault != "" || busy {
		return xerrors.New("OpenCode session is busy or under human control")
	}
	for _, request := range receipts.Requests {
		if openCodeRequestActive(request) {
			return xerrors.New("OpenCode request outcome remains pending or unknown")
		}
	}
	if len(receipts.Requests) >= 1000 {
		return xerrors.New("OpenCode request receipt budget exhausted; start a new session")
	}
	var random [13]byte
	_, _ = rand.Read(random[:])
	nativeID := "msg_" + hex.EncodeToString(random[:])
	request := &openCodeRequestReceipt{NativeID: nativeID, Hash: openCodeHash(prompt.Text), Status: "submitting"}
	receipts.Requests[prompt.ID] = request
	if err := o.publish(session.ID, nil, receipts); err != nil {
		o.fault(observation)
		return err
	}
	// A human takeover may happen while the ownership receipt is being saved.
	// Recheck admission after persistence, immediately before the sole dispatch.
	latest, latestErr := o.host.Session(session.ID)
	if latestErr != nil || latest.Mode != "automation" || latest.Archived || latest.ResumePending || latest.Fault != "" || ctx.Err() != nil {
		request.Status = "rejected"
		if err := o.publish(session.ID, []Event{{Type: "prompt.rejected", Data: map[string]any{"requestId": prompt.ID, "reason": "OpenCode admission changed before dispatch"}}}, receipts); err != nil {
			o.fault(observation)
			return err
		}
		return xerrors.New("OpenCode admission changed before dispatch")
	}
	var result struct {
		Data openCodeInboxItem `json:"data"`
	}
	err = o.client.request(ctx, "POST", "/api/session/"+session.NativeID+"/prompt", map[string]string{"id": nativeID, "text": prompt.Text, "delivery": "queue"}, &result)
	if err != nil {
		var rpcError *openCodeRPCError
		if errors.As(err, &rpcError) && rpcError.rejected {
			request.Status = "rejected"
			if publishErr := o.publish(session.ID, []Event{{Type: "prompt.rejected", Data: map[string]any{"requestId": prompt.ID, "reason": "Native OpenCode rejected input; inspect the native terminal"}}}, receipts); publishErr != nil {
				o.fault(observation)
				return publishErr
			}
		} else {
			o.fault(observation)
		}
		return err
	}
	if result.Data.ID != nativeID || result.Data.SessionID != session.NativeID || result.Data.Type != "user" {
		o.fault(observation)
		return errOpenCodeUnknown
	}
	request.Status = "queued"
	if err := o.publish(session.ID, nil, receipts); err != nil {
		o.fault(observation)
		return err
	}
	return nil
}

// Cancel can delete only an exactly owned pending inbox entry. It never calls
// the session interrupt endpoint, even if delivery races the deletion.
func (o *OpenCode) Cancel(ctx context.Context, session Session, requestID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	observation, err := o.current(session)
	if err != nil {
		return err
	}
	receipts, _, err := o.snapshot(ctx, observation, openCodeLiveSnapshot)
	if err != nil {
		o.fault(observation)
		return err
	}
	request := receipts.Requests[requestID]
	if request == nil {
		return xerrors.New("unknown OpenCode request")
	}
	if !openCodeRequestActive(request) {
		return nil
	}
	if request.Status != "queued" {
		return xerrors.New("OpenCode cannot safely cancel running or ambiguous input")
	}
	if request.CancelSent {
		return xerrors.New("OpenCode cancellation outcome is pending or unknown")
	}
	request.CancelSent = true
	request.CancelBy = o.clock.Now().Add(10 * time.Second).UnixMilli()
	if err := o.publish(session.ID, nil, receipts); err != nil {
		o.fault(observation)
		return err
	}
	if err := o.client.request(ctx, "DELETE", "/api/session/"+session.NativeID+"/inbox/"+request.NativeID, nil, nil); err != nil {
		o.fault(observation)
		return err
	}
	receipts, _, err = o.snapshot(ctx, observation, openCodeLiveSnapshot)
	if err != nil {
		o.fault(observation)
		return err
	}
	if receipts.Requests[requestID].Status == "running" {
		return xerrors.New("OpenCode input was already delivered; use the native terminal")
	}
	// Only the native cancellation event confirms success; absence from two
	// independently read snapshots is not an atomic proof of cancellation.
	return nil
}

// SetMode is a manager-only admission policy, not a native configuration change.
func (o *OpenCode) SetMode(ctx context.Context, session Session, mode string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !openCodeValidSession(session) || (mode != "human" && mode != "automation") {
		return xerrors.New("invalid OpenCode mode or session")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_, err := o.current(session)
	return err
}

// Attach only returns credentials after verifying the existing native identity,
// server ownership, and exact CLI version. It never starts a managed service.
func (o *OpenCode) Attach(ctx context.Context, session Session) (Command, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !openCodeValidSession(session) {
		return Command{}, xerrors.New("invalid OpenCode session identity")
	}
	path, err := o.server.version(ctx)
	if err != nil {
		return Command{}, err
	}
	descriptor, err := o.server.connect(ctx)
	if err != nil {
		return Command{}, err
	}
	client := &openCodeRPC{endpoint: descriptor.URL, password: descriptor.Password, http: o.server.http, clock: o.clock, ctx: ctx}
	var result struct {
		Data openCodeSessionInfo `json:"data"`
	}
	if err := client.request(ctx, "GET", "/api/session/"+session.NativeID, nil, &result); err != nil {
		return Command{}, err
	}
	if !result.Data.valid(session) {
		return Command{}, errOpenCodeProtocol
	}
	return Command{Path: path, Args: []string{"--server", descriptor.URL, "--session", session.NativeID, session.Cwd}, Dir: session.Cwd, Env: append(o.server.environment(), "OPENCODE_PASSWORD="+descriptor.Password)}, nil
}

// Close detaches HTTP observation. Native work and the private descriptor remain.
func (o *OpenCode) Close() error {
	o.cancel()
	o.mu.Lock()
	if o.client != nil {
		o.client.close()
		o.client = nil
	}
	o.mu.Unlock()
	o.wg.Wait()
	return nil
}

// Shutdown explicitly stops only the verified process named by our descriptor.
func (o *OpenCode) Shutdown(ctx context.Context) error {
	_ = o.Close()
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.server.shutdown(ctx)
}

type openCodeInboxItem struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionID"`
	Type      string `json:"type"`
	Payload   struct {
		Text *string `json:"text"`
	} `json:"payload"`
}

type openCodePendingItem struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionID"`
}

func (o *OpenCode) snapshot(ctx context.Context, observation *openCodeObservation, kind openCodeSnapshotKind) (*openCodeReceipts, bool, error) {
	session := observation.session
	saved, receipts, err := o.saved(session)
	if err != nil {
		return nil, false, err
	}
	root := "/api/session/" + session.NativeID
	var info struct {
		Data openCodeSessionInfo `json:"data"`
	}
	var page openCodeMessagePage
	var inbox struct {
		Data []openCodeInboxItem `json:"data"`
	}
	var active struct {
		Data map[string]struct {
			Type string `json:"type"`
		} `json:"data"`
	}
	var permissions, forms struct {
		Data []openCodePendingItem `json:"data"`
	}
	var shells struct {
		Data     []json.RawMessage `json:"data"`
		Location struct {
			Directory string `json:"directory"`
		} `json:"location"`
	}
	// These reads are bounded independently. Native idle markers, not the order
	// of these responses, are the sole completion authority.
	for _, read := range []struct {
		path   string
		target any
	}{
		{root, &info}, {root + "/message?limit=200&order=desc", &page}, {root + "/inbox", &inbox},
		{"/api/session/active", &active}, {root + "/permission", &permissions}, {root + "/form", &forms},
		{"/api/shell?location%5Bdirectory%5D=" + url.QueryEscape(session.Cwd), &shells},
	} {
		if err := o.client.request(ctx, http.MethodGet, read.path, nil, read.target); err != nil {
			return nil, false, err
		}
	}
	if !info.Data.valid(session) || !page.valid(200) || inbox.Data == nil || len(inbox.Data) > 1000 || active.Data == nil || permissions.Data == nil || forms.Data == nil || len(permissions.Data) > 128 || len(forms.Data) > 128 || shells.Data == nil || len(shells.Data) > 1000 || shells.Location.Directory != session.Cwd {
		return nil, false, errOpenCodeProtocol
	}
	if receipts.LastIdle == "" && page.Cursor.Next != nil {
		var older openCodeMessagePage
		if err := o.client.request(ctx, "GET", root+"/message?limit=1&cursor="+url.QueryEscape(*page.Cursor.Next), nil, &older); err != nil {
			return nil, false, err
		}
		if !older.valid(1) {
			return nil, false, errOpenCodeProtocol
		}
		if len(older.Data) != 0 {
			return nil, false, errOpenCodeAnchor
		}
		if older.Cursor.Previous != nil || older.Cursor.Next != nil {
			return nil, false, errOpenCodeProtocol
		}
	}
	events, background, err := receipts.project(page, kind)
	if err != nil {
		return nil, false, err
	}
	if kind == openCodeResumeSnapshot {
		eventKind := "runtime.resumed"
		if len(saved.State) == 0 {
			eventKind = "session.start"
		}
		events = append([]Event{{Type: eventKind, Data: map[string]any{"sessionId": session.ID, "runtimeId": session.NativeID, "mode": "human", "version": openCodeVersion}}}, events...)
	}
	for _, item := range inbox.Data {
		if item.SessionID != session.NativeID || !openCodeMessageID.MatchString(item.ID) || !slices.Contains([]string{"user", "synthetic", "compaction", "move"}, item.Type) {
			return nil, false, errOpenCodeProtocol
		}
		owner := receipts.owner(item.ID)
		if owner == "" && saved.Mode == "automation" {
			events = append(events, Event{Type: "mode", Data: map[string]any{"mode": "human"}})
			saved.Mode = "human"
		}
		if owner != "" {
			request := receipts.Requests[owner]
			if item.Type != "user" || item.Payload.Text == nil || openCodeHash(*item.Payload.Text) != request.Hash {
				return nil, false, errOpenCodeProtocol
			}
			// A matching inbox entry proves acceptance even if the sole POST
			// response was lost. Never downgrade running or terminal receipts.
			if request.Status == "submitting" {
				request.Status = "queued"
			}
		}
	}
	for _, request := range receipts.Requests {
		if request.CancelSent && request.Status == "queued" && request.CancelBy > 0 && o.clock.Now().UnixMilli() >= request.CancelBy {
			return nil, false, xerrors.New("OpenCode queued cancellation was not confirmed; inspect the native terminal")
		}
	}
	running, exists := active.Data[session.NativeID]
	if exists && running.Type != "running" {
		return nil, false, errOpenCodeProtocol
	}
	var approvals []string
	for _, list := range []struct {
		kind  string
		items []openCodePendingItem
	}{{"permission", permissions.Data}, {"form", forms.Data}} {
		for _, item := range list.items {
			if item.SessionID != session.NativeID || item.ID == "" || len(item.ID) > 200 {
				return nil, false, errOpenCodeProtocol
			}
			key := list.kind + ":" + item.ID
			approvals = append(approvals, key)
			if !slices.Contains(receipts.Approvals, key) {
				events = append(events, receipts.event("needs.input", receipts.Active, map[string]any{"kind": list.kind, "pending": true}))
			}
		}
	}
	for _, old := range receipts.Approvals {
		if !slices.Contains(approvals, old) {
			events = append(events, receipts.event("needs.input", receipts.Active, map[string]any{"kind": "native", "pending": len(approvals) > 0}))
			break
		}
	}
	receipts.Approvals = approvals
	if err := o.publish(session.ID, events, receipts); err != nil {
		return nil, false, err
	}
	return receipts, background || len(inbox.Data) > 0 || exists || len(shells.Data) > 0 || len(receipts.Turns) > 0 || len(approvals) > 0, nil
}

func openCodeEventSession(event openCodeNativeEvent) string {
	var id string
	_ = json.Unmarshal(event.Data["sessionID"], &id)
	if id == "" {
		var form struct {
			SessionID string `json:"sessionID"`
		}
		_ = json.Unmarshal(event.Data["form"], &form)
		id = form.SessionID
	}
	return id
}

// enqueue coalesces ordinary invalidations but retains destructive edges in
// arrival order. Its memory is bounded by registered sessions and 128 edges,
// independently of token throughput or the duration of an HTTP snapshot.
func (o *OpenCode) enqueue(generation uint64, event openCodeNativeEvent) bool {
	o.eventMu.Lock()
	defer o.eventMu.Unlock()
	if generation != o.pending.generation {
		return true
	}
	if event.Type == "transport.lost" {
		o.pending.lost = true
	} else {
		id := openCodeEventSession(event)
		if !o.eventSessions[id] {
			return true
		}
		if o.pending.dirty == nil {
			o.pending.dirty = map[string]bool{}
		}
		o.pending.dirty[id] = true
		switch event.Type {
		case "session.deleted", "session.revert.committed", "session.inbox.cancelled":
			if len(o.pending.critical) == 128 {
				o.pending.lost = true
			} else {
				o.pending.critical = append(o.pending.critical, event)
			}
		}
	}
	select {
	case o.wake <- struct{}{}:
	default:
	}
	return !o.pending.lost
}

func (o *OpenCode) drain() openCodeWake {
	o.eventMu.Lock()
	defer o.eventMu.Unlock()
	pending := o.pending
	o.pending = openCodeWake{generation: pending.generation}
	select {
	case <-o.wake:
	default:
	}
	return pending
}

func (o *OpenCode) observe() {
	defer o.wg.Done()
	ticker := o.clock.NewTicker(time.Second, "opencode.poll")
	defer ticker.Stop()
	for {
		poll := false
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			poll = true
		case <-o.wake:
		}
		o.mu.Lock()
		if o.ctx.Err() != nil {
			o.mu.Unlock()
			return
		}
		wake := o.drain()
		if wake.generation != o.generation {
			o.mu.Unlock()
			continue
		}
		// Apply edges before snapshots, in native arrival order. Ordinary text
		// deltas cannot displace a cancellation or hide a deletion/revert.
		for _, event := range wake.critical {
			observation := o.sessions["opencode:"+openCodeEventSession(event)]
			if observation == nil || !observation.ready || observation.fault {
				continue
			}
			switch event.Type {
			case "session.deleted", "session.revert.committed":
				o.fault(observation)
			case "session.inbox.cancelled":
				if err := o.canceled(observation, event); err != nil {
					o.fault(observation)
				}
			}
		}
		for _, observation := range o.sessions {
			if !observation.ready || observation.fault {
				continue
			}
			if wake.lost || o.client == nil || o.client.ctx.Err() != nil {
				o.fault(observation)
				continue
			}
			if !poll && !wake.dirty[observation.session.NativeID] {
				continue
			}
			if _, _, err := o.snapshot(o.ctx, observation, openCodeLiveSnapshot); err != nil {
				o.fault(observation)
			}
		}
		o.mu.Unlock()
	}
}

func (o *OpenCode) canceled(observation *openCodeObservation, event openCodeNativeEvent) error {
	var nativeID string
	if json.Unmarshal(event.Data["inboxID"], &nativeID) != nil || !openCodeMessageID.MatchString(nativeID) {
		return errOpenCodeProtocol
	}
	_, receipts, err := o.saved(observation.session)
	if err != nil {
		return err
	}
	owner := receipts.owner(nativeID)
	if owner == "" {
		return nil
	}
	request := receipts.Requests[owner]
	if !openCodeRequestActive(request) {
		return nil
	}
	request.Status = openCodeCanceledStatus
	//nolint:misspell // The normalized event contract uses this spelling.
	return o.publish(observation.session.ID, []Event{{Type: "prompt.rejected", Data: map[string]any{"requestId": owner, "cancelled": true, "reason": "Native OpenCode inbox input canceled"}}}, receipts)
}
