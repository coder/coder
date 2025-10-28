# Agent Socket API

The Agent Socket API provides a local communication channel between CLI commands running within a workspace and the Coder agent process. This enables new CLI commands to interact directly with the agent without going through the control plane.

## Overview

The socket server runs within the agent process and listens on a Unix domain socket (or named pipe on Windows). CLI commands can connect to this socket to query agent information, check health status, and perform other operations.

## Architecture

### Socket Server
- **Location**: `agent/agentsocket/`
- **Protocol**: JSON-RPC 2.0 over Unix domain socket
- **Platform Support**: Linux, macOS, Windows 10+ (build 17063+)
- **Authentication**: Pluggable middleware (no-auth by default)

### Client Library
- **Location**: `codersdk/agentsdk/socket_client.go`
- **Auto-discovery**: Automatically finds socket path
- **Type-safe**: Go client with proper error handling

## Socket Path Discovery

The socket path is determined in the following order:

1. **Environment Variable**: `CODER_AGENT_SOCKET_PATH`
2. **XDG Runtime Directory**: `$XDG_RUNTIME_DIR/coder-agent.sock`
3. **User Temp Directory**: `/tmp/coder-agent-{uid}.sock`
4. **Fallback**: `/tmp/coder-agent.sock`

## Protocol

### Request Format
```json
{
  "version": "1.0",
  "method": "ping",
  "id": "request-123",
  "params": {}
}
```

### Response Format
```json
{
  "version": "1.0",
  "id": "request-123",
  "result": {
    "message": "pong",
    "timestamp": "2024-01-01T00:00:00Z"
  }
}
```

### Error Format
```json
{
  "version": "1.0",
  "id": "request-123",
  "error": {
    "code": -32601,
    "message": "Method not found",
    "data": "nonexistent"
  }
}
```

## Available Methods

### Core Methods
- `ping` - Health check with timestamp
- `health` - Agent status and uptime
- `agent.info` - Detailed agent information
- `methods.list` - List available methods

### Example Usage

```go
// Create client
client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{})
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Ping the agent
pingResp, err := client.Ping(ctx)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Agent responded: %s\n", pingResp.Message)

// Get agent info
info, err := client.AgentInfo(ctx)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Agent ID: %s, Version: %s\n", info.ID, info.Version)
```

## Adding New Handlers

### Server Side
```go
// Register a new handler
server.RegisterHandler("custom.method", func(ctx Context, req *Request) (*Response, error) {
    // Handle the request
    result := map[string]string{"status": "ok"}
    return NewResponse(req.ID, result)
})
```

### Client Side
```go
// Add method to client
func (c *SocketClient) CustomMethod(ctx context.Context) (*CustomResponse, error) {
    req := &Request{
        Version: "1.0",
        Method:  "custom.method",
        ID:      generateRequestID(),
    }

    resp, err := c.sendRequest(ctx, req)
    if err != nil {
        return nil, err
    }

    if resp.Error != nil {
        return nil, fmt.Errorf("custom method error: %s", resp.Error.Message)
    }

    var result CustomResponse
    if err := json.Unmarshal(resp.Result, &result); err != nil {
        return nil, fmt.Errorf("unmarshal response: %w", err)
    }

    return &result, nil
}
```

## Authentication

The socket server supports pluggable authentication middleware. By default, no authentication is performed (suitable for local-only communication).

### Custom Authentication
```go
type CustomAuthMiddleware struct {
    // Add auth fields
}

func (m *CustomAuthMiddleware) Authenticate(ctx context.Context, conn net.Conn) (context.Context, error) {
    // Implement authentication logic
    // Return context with auth info or error
    return ctx, nil
}

// Use in server config
server := agentsocket.NewServer(agentsocket.Config{
    Path:           socketPath,
    Logger:         logger,
    AuthMiddleware: &CustomAuthMiddleware{},
})
```

## Configuration

### Agent Options
```go
options := agent.Options{
    // ... other options
    SocketPath: "/custom/path/agent.sock", // Optional, uses auto-discovery if empty
}
```

### Environment Variables
- `CODER_AGENT_SOCKET_PATH` - Override socket path
- `XDG_RUNTIME_DIR` - Used for socket path discovery

## Error Codes

| Code | Description |
|------|-------------|
| -32700 | Parse error |
| -32600 | Invalid request |
| -32601 | Method not found |
| -32602 | Invalid params |
| -32603 | Internal error |

## Platform Support

### Unix-like Systems (Linux, macOS)
- Uses Unix domain sockets
- Socket file permissions: 600 (owner read/write only)
- Auto-cleanup on shutdown

### Windows
- Uses Unix domain sockets (Windows 10 build 17063+)
- Falls back to named pipes if needed
- Simplified permission handling

## Security Considerations

1. **Local Only**: Socket is only accessible from within the workspace
2. **File Permissions**: Socket file is restricted to owner only
3. **No Network Access**: Unix domain sockets don't traverse network
4. **Authentication Ready**: Middleware pattern allows future auth implementation

## Future Extensibility

The design supports:
- **Protocol Versioning**: Request includes version field
- **Multiple Transports**: Interface-based design allows TCP/WebSocket later
- **Auth Plugins**: Middleware pattern for various auth methods
- **Custom Handlers**: Simple registration pattern for new commands
