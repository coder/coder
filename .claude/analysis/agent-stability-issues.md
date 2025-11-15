# Agent Stability Analysis

## Problem Description

Agents are showing **yellow status** on the web UI and **VS Code/Windsurf connections are dropping** frequently.

## Root Cause Analysis

### Current Timeout Configuration

Located in `coderd/coderd.go:368-378`:

```go
AgentConnectionUpdateFrequency = 15 seconds  // How often agent sends heartbeat
AgentInactiveDisconnectTimeout = 30 seconds  // 2x update frequency = disconnect threshold
```

### Issues Identified

1. **30-second disconnect timeout is TOO SHORT** for production environments
   - Network latency can easily exceed this
   - Server under load may delay processing
   - DERP relay issues can cause temporary delays

2. **Heartbeat collision**
   - WebSocket heartbeat: 15 seconds (`coderd/httpapi/websocket.go:14`)
   - Agent connection update: 15 seconds
   - These can interfere with each other

3. **No retry mechanism**
   - Single failed heartbeat can trigger disconnect
   - No grace period for transient network issues

4. **Test vs Production mismatch**
   - Tests use `testutil.WaitShort` (more conservative)
   - Production uses hardcoded 30 seconds

### Evidence

**File:** `coderd/workspaceagents.go:1280`
```go
// TODO(mafredri): Is this too frequent? Use separate ping disconnect timeout?
```
^^ Developer already noted this might be problematic!

## Recommended Fixes

### Option 1: Increase Timeouts (Quick Fix)

```go
// In coderd/coderd.go
if options.AgentConnectionUpdateFrequency == 0 {
    options.AgentConnectionUpdateFrequency = 30 * time.Second  // Was: 15s
}
if options.AgentInactiveDisconnectTimeout == 0 {
    options.AgentInactiveDisconnectTimeout = options.AgentConnectionUpdateFrequency * 4  // Was: *2
    // Minimum 60 seconds instead of 2
    if options.AgentInactiveDisconnectTimeout < 60*time.Second {
        options.AgentInactiveDisconnectTimeout = 60 * time.Second
    }
}
```

**Result:** 30s heartbeat, 120s disconnect timeout (4x buffer)

### Option 2: Make Configurable (Better Long-term)

Add environment variables:
```bash
CODER_AGENT_UPDATE_FREQUENCY=30s
CODER_AGENT_DISCONNECT_TIMEOUT=120s
```

### Option 3: Separate Ping and Health Check

Implement the TODO from line 1280:
- Use websocket ping for keep-alive (15s)
- Use separate health check for agent status (30s+)
- Use longer disconnect timeout (120s+)

## Impact Assessment

### Current Impact
- **User Experience:** Poor - frequent disconnects
- **VS Code/Windsurf:** Unusable for long sessions
- **Network Overhead:** High - constant reconnections

### After Fix
- **User Experience:** Smooth - rare disconnects only on real issues
- **VS Code/Windsurf:** Stable connections
- **Network Overhead:** Lower - fewer reconnections

## Testing Requirements

1. Test with high latency network (> 50ms)
2. Test with server under load
3. Test long-running VS Code sessions (> 1 hour)
4. Test DERP relay scenarios
5. Ensure graceful degradation

## Related Files

- `coderd/coderd.go:368-378` - Timeout configuration
- `coderd/workspaceagents.go:1280` - Connection update loop
- `coderd/workspaceagentsrpc.go:235-239` - Agent RPC setup
- `coderd/httpapi/websocket.go:14` - WebSocket heartbeat
- `tailnet/` - DERP and networking layer

## Additional Observations

- DERP server health not monitored adequately
- No metrics for heartbeat failures before disconnect
- No configurable retry logic

## Priority

**HIGH** - Directly impacts developer productivity and core functionality.
