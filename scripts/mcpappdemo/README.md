# mcpappdemo

A small MCP server for trying MCP Apps (SEP-1865) in Coder Agents. It serves
one interactive view, a dice board, at `ui://mcpappdemo/dice.html`.

Run it:

```sh
go run ./scripts/mcpappdemo --addr 127.0.0.1:8811
```

It prints `mcpappdemo: MCP at http://127.0.0.1:8811/mcp`. Add that URL as an
MCP server in Coder. Each client initialize logs the client name and its
`capabilities.extensions`.

coderd refuses private addresses by default. To reach a loopback server, start
coderd with `CODER_MCP_ALLOWED_PRIVATE_CIDRS=127.0.0.1/32`.

Tools:

- `show_dice_board` rolls dice and opens the dice board. It is also a thread entry point.
- `roll_dice` is visible only to the view. The "Roll again" button calls it.
- `get_server_time` is visible only to the model. The view's "Server time" button calls it, and the host must refuse that call.
