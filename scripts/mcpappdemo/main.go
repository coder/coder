// Command mcpappdemo is a small MCP server for trying MCP Apps (SEP-1865)
// in Coder Agents. It serves one interactive view, a dice board, plus tools
// that exercise the app-only and model-only visibility rules.
package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/xerrors"
)

const (
	diceURI     = "ui://mcpappdemo/dice.html"
	appMIMEType = "text/html;profile=mcp-app"
)

//go:embed dice.html
var diceHTML string

// diceArgs is the input of show_dice_board and roll_dice. Zero values mean
// the field was omitted, so the tools accept {}.
type diceArgs struct {
	Count int `json:"count,omitempty"`
	Sides int `json:"sides,omitempty"`
}

// diceResult is the structured content of a dice roll.
type diceResult struct {
	Count int   `json:"count"`
	Sides int   `json:"sides"`
	Rolls []int `json:"rolls"`
	Total int   `json:"total"`
}

type timeResult struct {
	Time string `json:"time"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8811", "address to listen on")
	flag.Parse()

	server := newServer()
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("mcpappdemo: listen: %v", err)
	}
	_, _ = fmt.Printf("mcpappdemo: MCP at http://%s/mcp\n", ln.Addr())
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.Serve(ln); err != nil {
		log.Fatalf("mcpappdemo: serve: %v", err)
	}
}

func newServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "mcpappdemo", Version: "0.1.0"}, &mcp.ServerOptions{
		InitializedHandler: logInitialize,
	})

	server.AddResource(&mcp.Resource{
		URI:         diceURI,
		Name:        "dice",
		Title:       "Dice board",
		Description: "Interactive dice board view.",
		MIMEType:    appMIMEType,
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		if req.Params.URI != diceURI {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      diceURI,
			MIMEType: appMIMEType,
			Text:     diceHTML,
		}}}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "show_dice_board",
		Title:       "Dice board",
		Description: "Roll dice and show them on an interactive dice board.",
		InputSchema: diceSchema(),
		Meta: mcp.Meta{
			"ui":        map[string]any{"resourceUri": diceURI},
			"openai/ui": map[string]any{"entrypoints": []any{map[string]any{"type": "thread"}}},
		},
	}, rollTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "roll_dice",
		Description: "Roll again from the dice board view.",
		InputSchema: diceSchema(),
		Meta: mcp.Meta{
			"ui": map[string]any{"resourceUri": diceURI, "visibility": []string{"app"}},
		},
	}, rollTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_server_time",
		Description: "Get the demo server's current time.",
		Meta: mcp.Meta{
			"ui": map[string]any{"visibility": []string{"model"}},
		},
	}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, timeResult, error) {
		now := time.Now().Format(time.RFC3339)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: now}}}, timeResult{Time: now}, nil
	})

	return server
}

// logInitialize prints the client name and its advertised extensions, so a
// reviewer can confirm that the host sent io.modelcontextprotocol/ui.
func logInitialize(_ context.Context, req *mcp.InitializedRequest) {
	params := req.Session.InitializeParams()
	if params == nil {
		log.Printf("mcpappdemo: client initialized without params")
		return
	}
	name := "<unknown>"
	if params.ClientInfo != nil {
		name = params.ClientInfo.Name
	}
	var extensions map[string]any
	if params.Capabilities != nil {
		extensions = params.Capabilities.Extensions
	}
	encoded, err := json.Marshal(extensions)
	if err != nil {
		encoded = []byte(fmt.Sprintf("%q", err.Error()))
	}
	log.Printf("mcpappdemo: client %q initialized, extensions=%s", name, encoded)
}

func diceSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"count": {
				Type:        "integer",
				Description: "Number of dice to roll.",
				Minimum:     jsonschema.Ptr(1.0),
				Maximum:     jsonschema.Ptr(10.0),
				Default:     json.RawMessage("2"),
			},
			"sides": {
				Type:        "integer",
				Description: "Number of sides on each die.",
				Minimum:     jsonschema.Ptr(2.0),
				Maximum:     jsonschema.Ptr(100.0),
				Default:     json.RawMessage("6"),
			},
		},
	}
}

func rollTool(_ context.Context, _ *mcp.CallToolRequest, args diceArgs) (*mcp.CallToolResult, diceResult, error) {
	if args.Count == 0 {
		args.Count = 2
	}
	if args.Sides == 0 {
		args.Sides = 6
	}
	if args.Count < 1 || args.Count > 10 || args.Sides < 2 || args.Sides > 100 {
		return nil, diceResult{}, xerrors.New("count must be 1..10 and sides must be 2..100")
	}

	res := diceResult{Count: args.Count, Sides: args.Sides, Rolls: make([]int, 0, args.Count)}
	texts := make([]string, 0, args.Count)
	for range args.Count {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(args.Sides)))
		if err != nil {
			return nil, diceResult{}, xerrors.Errorf("roll die: %w", err)
		}
		roll := int(n.Int64()) + 1
		res.Rolls = append(res.Rolls, roll)
		res.Total += roll
		texts = append(texts, strconv.Itoa(roll))
	}

	text := fmt.Sprintf("Rolled %dd%d: %s (total %d)", res.Count, res.Sides, strings.Join(texts, ", "), res.Total)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, res, nil
}
