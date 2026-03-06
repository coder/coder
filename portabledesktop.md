# Portable Desktop Streaming: Implementation Spec

## Overview

Add a new endpoint `GET /api/experimental/chats/{chat}/desktop` that lets
the frontend subscribe over WebSocket to a live desktop stream from a
[portabledesktop](https://github.com/coder/portabledesktop) session running
inside a workspace. The browser receives raw RFB (VNC) frames and can send
keyboard/mouse input back — the same protocol portabledesktop already uses.

The data path is:

```
Browser (noVNC client)
    │  WebSocket (binary RFB frames)
    ▼
coderd: GET /api/experimental/chats/{chat}/desktop
    │  Bicopy raw bytes
    ▼
ServerTailnet → AgentConn
    │  WebSocket over tailnet HTTP (port 4)
    ▼
Agent: GET /api/v0/desktop
    │  TCP → 127.0.0.1:<vncPort>
    ▼
Xvnc (portabledesktop session, one per agent)
```

coderd and the agent never inspect the bytes — they shovel them
transparently. The browser's noVNC client and Xvnc speak RFB directly
through the tunnel.

## Protocol: RFB over WebSocket

The wire protocol is **RFB (Remote Framebuffer Protocol, RFC 6143)** — the
standard VNC protocol. All bytes flow unmodified through the WebSocket
tunnel.

### Handshake (sequential, happens once per connection)

1. **ProtocolVersion** — server sends `"RFB 003.008\n"`, client echoes.
2. **Security** — server offers types, client selects (`None`).
3. **SecurityResult** — server confirms.
4. **ServerInit** — server sends framebuffer width, height, pixel format,
   desktop name.

### Steady-state messages

Client → server:

| Type                     | ID | Purpose                                   |
|--------------------------|----|-------------------------------------------|
| `SetPixelFormat`         | 0  | Request specific pixel encoding           |
| `SetEncodings`           | 2  | Advertise supported encodings             |
| `FramebufferUpdateRequest` | 3 | Request a region (incremental or full)   |
| `KeyEvent`               | 4  | Key press/release (X11 keysym + down flag)|
| `PointerEvent`           | 5  | Mouse position + button mask              |
| `ClientCutText`          | 6  | Clipboard paste                           |

Server → client:

| Type                     | ID | Purpose                                   |
|--------------------------|----|-------------------------------------------|
| `FramebufferUpdate`      | 0  | N rectangles of pixel data                |
| `SetColourMapEntries`    | 1  | Palette updates (rare)                    |
| `Bell`                   | 2  | Audio bell                                |
| `ServerCutText`          | 3  | Clipboard from server                     |

`FramebufferUpdate` is the critical rendering message. Each contains
rectangles of `(x, y, width, height, encoding, pixels)`. noVNC supports
Raw, CopyRect, Tight (JPEG), ZRLE, and Hextile encodings.

Multiple browser clients can connect simultaneously. portabledesktop uses
`shared: true` by default, which sends a shared-flag byte during
`ClientInit` telling Xvnc not to disconnect existing clients.

---

## Layer 1: coderd — Route and Proxy

### Route registration

File: `coderd/coderd.go`, inside the `/{chat}` route group (~line 1132).
Add alongside the existing `git/watch` route:

```go
r.Get("/desktop", api.watchChatDesktop)
```

This inherits the `ExtractChatParam` middleware, so `httpmw.ChatParam(r)`
is available in the handler.

### Handler: `watchChatDesktop`

File: `coderd/chats.go`. Follow the **PTY proxy pattern**
(`coderd/workspaceapps/proxy.go:705-803`), not the git/watch pattern,
because this is a raw binary stream.

Steps:

1. Extract chat, validate `chat.WorkspaceID` is set.
2. Fetch agents via `GetWorkspaceAgentsInLatestBuildByWorkspaceID`,
   take `agents[0]`, confirm `apiAgent.Status == Connected`.
3. Dial the agent over tailnet:
   ```go
   dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
   defer dialCancel()
   agentConn, release, err := api.agentProvider.AgentConn(dialCtx, agents[0].ID)
   defer release()
   ```
4. Dial the agent's desktop WebSocket — this returns a `net.Conn`
   carrying raw RFB bytes:
   ```go
   desktopConn, err := agentConn.Desktop(ctx)
   defer desktopConn.Close()
   ```
5. Accept the browser's WebSocket with **compression disabled** (binary
   stream, compression would add latency for no benefit):
   ```go
   conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
       CompressionMode: websocket.CompressionDisabled,
   })
   ```
6. Wrap as `net.Conn` and bicopy:
   ```go
   ctx, wsNetConn := workspaceapps.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
   defer wsNetConn.Close()
   go httpapi.HeartbeatClose(ctx, logger, cancel, conn)
   agentssh.Bicopy(ctx, wsNetConn, desktopConn)
   ```

The full handler is structurally identical to `workspaceAgentPTY` in
`coderd/workspaceapps/proxy.go:705-803`, minus the signed-app-token auth
(chat endpoints use apikey middleware) and the PTY-specific init params.

---

## Layer 2: SDK — `AgentConn.Desktop()`

File: `codersdk/workspacesdk/agentconn.go`. Add a new method on
`agentConn`, modeled on `WatchGit` (~line 493) but returning a raw
`net.Conn` instead of a `wsjson.Stream`:

```go
// Desktop opens a WebSocket to the agent's desktop endpoint and
// returns a net.Conn carrying raw RFB (VNC) binary data.
func (c *agentConn) Desktop(ctx context.Context) (net.Conn, error) {
    ctx, span := tracing.StartSpan(ctx)
    defer span.End()

    host := net.JoinHostPort(
        c.agentAddress().String(),
        strconv.Itoa(AgentHTTPAPIServerPort),
    )

    dialOpts := &websocket.DialOptions{
        HTTPClient:      c.apiClient(),
        CompressionMode: websocket.CompressionDisabled,
    }
    c.headersMu.RLock()
    if len(c.extraHeaders) > 0 {
        dialOpts.HTTPHeader = c.extraHeaders.Clone()
    }
    c.headersMu.RUnlock()

    url := fmt.Sprintf("http://%s/api/v0/desktop", host)
    conn, res, err := websocket.Dial(ctx, url, dialOpts)
    if err != nil {
        if res == nil {
            return nil, err
        }
        return nil, codersdk.ReadBodyAsError(res)
    }
    if res != nil && res.Body != nil {
        defer res.Body.Close()
    }

    // No read limit — RFB framebuffer updates can be large.
    conn.SetReadLimit(-1)

    return websocket.NetConn(ctx, conn, websocket.MessageBinary), nil
}
```

Key difference from `WatchGit`: uses `websocket.MessageBinary` (not
`MessageText`), `CompressionDisabled` (not `NoContextTakeover`), and
returns a raw `net.Conn` (not a `wsjson.Stream`). No `chatID` parameter
is needed because the desktop session is a singleton per agent — it does
not vary by chat.

---

## Layer 3: Agent — `agentdesktop` Package

### Package structure

Create `agent/agentdesktop/` with a single file `api.go`, following the
pattern of `agent/agentgit/`, `agent/agentcontainers/`, etc.

### API struct

```go
package agentdesktop

type API struct {
    logger  slog.Logger

    mu      sync.Mutex
    session *desktopSession // nil until first connection
    closed  bool
}

type desktopSession struct {
    cmd     *exec.Cmd
    vncPort int
    cancel  context.CancelFunc
}
```

### Initialization

In `agent/agent.go`, alongside the existing sub-API creation (~line 385):

```go
a.desktopAPI = agentdesktop.NewAPI(a.logger.Named("desktop"))
```

Add a field to the `agent` struct (~line 307):

```go
desktopAPI *agentdesktop.API
```

### Route mount

In `agent/api.go` (~line 31), add:

```go
r.Mount("/api/v0/desktop", a.desktopAPI.Routes())
```

### Routes

```go
func (a *API) Routes() http.Handler {
    r := chi.NewRouter()
    r.Get("/", a.handleDesktop)
    return r
}
```

### Handler: `handleDesktop`

This is the core of the agent-side implementation:

1. **Check for the binary.** Look up `portabledesktop` in PATH via
   `exec.LookPath`. If missing, return `424 Failed Dependency` with
   a message explaining the binary is not installed.

2. **Get or create the singleton session.** Under `api.mu`:
   - If `api.session != nil`, reuse it (check the process is still alive
     first — if it died, nil it out and recreate).
   - If `api.session == nil`, spawn `portabledesktop up --json`. Parse
     the JSON stdout to extract `vncPort`. Store the `*exec.Cmd` and
     port in `api.session`.
   - The session persists for the agent's lifetime. One Xvnc per
     workspace, shared by all connected browsers.

3. **Accept WebSocket** from coderd:
   ```go
   conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
       CompressionMode: websocket.CompressionDisabled,
   })
   ```

4. **Dial Xvnc** over local TCP:
   ```go
   tcp, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", session.vncPort))
   ```

5. **Bicopy.** Wrap the WebSocket as a `net.Conn` and shovel bytes:
   ```go
   ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
   agentssh.Bicopy(ctx, wsNetConn, tcp)
   ```

Each new browser connection gets its own WebSocket + TCP socket pair, but
they all connect to the same Xvnc process. RFB's shared-flag allows this
natively.

### Cleanup

```go
func (a *API) Close() error {
    a.mu.Lock()
    defer a.mu.Unlock()
    a.closed = true
    if a.session != nil {
        a.session.cancel()
        // Xvnc is a child process — killing it cleans up the X session.
        _ = a.session.cmd.Process.Kill()
        _ = a.session.cmd.Wait()
        a.session = nil
    }
    return nil
}
```

Wire into agent shutdown in `agent/agent.go` (~line 2056):

```go
if err := a.desktopAPI.Close(); err != nil {
    a.logger.Error(a.hardCtx, "desktop API close", slog.Error(err))
}
```

---

## Frontend Integration

The frontend connects using noVNC's `RFB` class (or portabledesktop's
`createClient` wrapper). The WebSocket URL is:

```
ws[s]://<coder-host>/api/experimental/chats/<chatId>/desktop
```

Example usage with portabledesktop's client library:

```ts
import { createClient } from "portabledesktop/client";

const protocol = location.protocol === "https:" ? "wss" : "ws";
const wsUrl = `${protocol}://${location.host}/api/experimental/chats/${chatId}/desktop`;

const rfb = createClient(document.getElementById("desktop-viewer")!, {
  url: wsUrl,
  scaleViewport: true,
  shared: true,
});

rfb.addEventListener("connect", () => console.log("Desktop connected"));
rfb.addEventListener("disconnect", (e) => console.log("Disconnected", e.detail));
```

Or directly with `@novnc/novnc`:

```ts
import RFB from "@novnc/novnc/core/rfb";

const rfb = new RFB(
  document.getElementById("desktop-viewer")!,
  `${protocol}://${location.host}/api/experimental/chats/${chatId}/desktop`,
);
rfb.scaleViewport = true;
```

The frontend should handle the `disconnect` event to show reconnect UI,
and may want to set `rfb.viewOnly = true` for spectator-mode viewing.

---

## Error Cases

| Scenario | Layer | Response |
|---|---|---|
| Chat has no `workspace_id` | coderd | 400 "Chat has no workspace." |
| No agents in workspace | coderd | 400 "Chat workspace has no agents." |
| Agent not connected | coderd | 400 "Agent state is X, must be connected." |
| Tailnet dial fails | coderd | 500 "Failed to dial workspace agent." |
| `portabledesktop` not in PATH | agent | 424 "portabledesktop binary not found." |
| `portabledesktop up` fails to start | agent | 500 "Failed to start desktop session." |
| Xvnc process died mid-session | agent | Recreate session on next connection; existing connections get a WebSocket close frame. |

---

## Security

- The coderd endpoint is gated by `apiKeyMiddleware` and
  `RequireExperimentWithDevBypass(ExperimentAgents)`, same as all
  other chat endpoints.
- The agent-side endpoint is on tailnet port 4, only reachable via
  coderd's `ServerTailnet`. It is not exposed to the public internet.
- Xvnc runs with `-SecurityTypes None` (no VNC auth) because it only
  listens on `127.0.0.1` inside the workspace. Access control is
  handled by coderd's auth layer, not VNC.

---

## Out of Scope

- **Desktop resize from the browser.** The DesktopSize pseudo-encoding
  could be supported later but is not required for the initial
  implementation. Use a fixed geometry (e.g., `1280x800`).
- **Multiple desktops per workspace.** One Xvnc session per agent
  process is sufficient. Multi-desktop support can be added later if
  needed.
- **Recording/screenshots.** portabledesktop supports `ffmpeg`-based
  recording, but that is an AI-agent concern, not a browser-streaming
  concern.
- **Audio.** RFB has no audio channel. Out of scope.

---

## Implementation TODOs

- [x] **SDK: Add `Desktop()` method to `AgentConn`.**
  File: `codersdk/workspacesdk/agentconn.go`. Add a method that dials
  `ws://<agent>:4/api/v0/desktop`, returns a `net.Conn` carrying raw
  RFB binary data. Use `websocket.MessageBinary` and
  `CompressionDisabled`. No read limit.

- [x] **Agent: Create `agent/agentdesktop/` package.**
  New file: `agent/agentdesktop/api.go`. Define the `API` struct with a
  `sync.Mutex`-guarded singleton `desktopSession`, `NewAPI()` constructor,
  `Routes()` returning a chi router, and `Close()` for cleanup.

- [x] **Agent: Implement `handleDesktop` WebSocket handler.**
  In `agent/agentdesktop/api.go`. Check for `portabledesktop` binary via
  `exec.LookPath`. Get-or-create the singleton Xvnc session by running
  `portabledesktop up --json` and parsing the VNC port from stdout.
  Accept WebSocket, dial `127.0.0.1:<vncPort>` over TCP, and
  `agentssh.Bicopy` the two connections.

- [x] **Agent: Wire up `desktopAPI` in agent lifecycle.**
  In `agent/agent.go`: add `desktopAPI *agentdesktop.API` field (~line
  307). Create it in the init block (~line 385). Call
  `a.desktopAPI.Close()` in the shutdown path (~line 2056).

- [x] **Agent: Mount route in agent HTTP API.**
  In `agent/api.go` (~line 31): add
  `r.Mount("/api/v0/desktop", a.desktopAPI.Routes())`.

- [x] **coderd: Register the `/desktop` route.**
  In `coderd/coderd.go` inside the `/{chat}` route group (~line 1135):
  add `r.Get("/desktop", api.watchChatDesktop)`.

- [x] **coderd: Implement `watchChatDesktop` handler.**
  In `coderd/chats.go`. Extract chat, validate workspace/agent, dial
  agent via `agentConn.Desktop(ctx)`, accept browser WebSocket with
  `CompressionDisabled` and `MessageBinary`, then
  `agentssh.Bicopy(ctx, wsNetConn, desktopConn)`. Model on the PTY
  handler at `coderd/workspaceapps/proxy.go:705-803`.

- [x] **coderd: Add Swagger annotations for the new endpoint.**
  Add `@Summary`, `@ID`, `@Tags`, `@Param`, `@Success 101`, and
  `@Router` comments above `watchChatDesktop`, following the pattern of
  other chat endpoints in `coderd/chats.go`.

- [x] **Tests: Agent-side unit test.**
  In `agent/agentdesktop/api_test.go`. Test that `handleDesktop` returns
  424 when `portabledesktop` is not in PATH. Test that the singleton
  session is reused across multiple connections. Test that `Close()`
  kills the Xvnc process.

- [x] **Tests: coderd integration test.**
  In `coderd/chats_test.go` or a new file. Test the full WebSocket
  upgrade flow: create a chat with a workspace, connect to
  `/api/experimental/chats/{chat}/desktop`, verify the WebSocket
  upgrades successfully and bytes flow end-to-end.

- [x] **Frontend: Add desktop viewer component.**
  Add a React component that takes a `chatId`, constructs the WebSocket
  URL (`/api/experimental/chats/${chatId}/desktop`), and renders an
  noVNC `RFB` instance into a container div. Handle `connect`,
  `disconnect`, and `credentialsrequired` events.
