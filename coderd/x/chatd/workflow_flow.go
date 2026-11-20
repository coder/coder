package chatd

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"

	"cel.dev/cel-go/cel"
	celast "cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/common/types/traits"
	"cel.dev/cel-go/ext"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	runFlowToolName = "run_flow"

	maxFlowExprSize = 4096      // code points in one CEL expression
	maxFlowExprCost = 1_000_000 // CEL cost units in one evaluation
	// flowInterruptCheckFrequency is how many comprehension iterations
	// run between context checks.
	flowInterruptCheckFrequency = 100
)

const runFlowDescription = `Start a flow run: a JSON graph of sub-agent steps. The runtime walks the graph. Small CEL expressions compute the dynamic parts. ` +
	`The tool returns at once with a run_id and status "running". The run continues in the background. ` +
	`Call await_workflow with the run_id to get the result. If your turn ends first, you get a message when the run ends. Then call await_workflow.

Send the flow inline in "flow", or write it to a JSON file in the workspace and send its absolute path in "flow_path". Send one of the two. ` +
	`Any program in the workspace can write the file, for example a Python or Node script. ` +
	`chatd reads the file one time, when the run starts. ` +
	`run_flow runs after the other tool calls in the same batch, so a write of the file in that batch is done first.

Flow fields:
- start: the ID of the first node.
- nodes: an object that maps a node ID to a node. An ID matches ^[a-z][a-z0-9_]*$.
- edges: a list of {"from", "to", "when"}. when is a CEL bool. An edge without when always matches.
- output: CEL for the run result. Without output, the result is the value of the last node visit.

Node fields:
- prompt (required) and title: text with {{ CEL }} parts. A string result goes in as is. Any other result goes in as compact JSON. Write {{ "{{" }} for a literal {{. The default title is the node ID, plus [i] for item i and #n for visit n.
- type: "general" (default) or "explore".
- schema: a JSON Schema object with "type": "object" at the top level. ` +
	`The step must call its workflow_report tool with a value that matches the schema. That value is the node value. Without a schema the value is {"text": "..."}.
- for_each: CEL that returns a list. The node starts one step per item, all at once. Its value is the list of item values, in item order. A failed item gives null.
- max_visits: how many times the node can run. Default 1.

CEL variables: input is the "args" object. nodes.<id>.value is the latest value of a node, and nodes.<id>.values lists all its values. ` +
	`A node that has not run has value null. item and index exist in the prompt and title of a for_each node. value is the value of the "from" node, in when. ` +
	`JSON numbers are doubles. Comparisons with ints work. In arithmetic, write 1.0 instead of 1, or use int(). ` +
	`Extensions: strings, lists, math, sets, base64, optional values and two-variable comprehensions.

Execution: the run visits start. After each visit it takes the first edge from that node, in edges order, whose when is empty or true. ` +
	`No matching edge ends the run. An edge back to an earlier node makes a loop. A visit past max_visits fails the run. ` +
	`A failed step of a node without for_each fails the run.

The flow is checked before any step runs. The error lists every problem, one per line. Fix them all, then call run_flow again. ` +
	`Limits: flow 64 KiB, 50 steps per run, result 64 KiB. Each step is a child chat of this chat. ` +
	`chatd limits how many sub-agent chats run at the same time. Extra steps wait in a queue.

Example, review files in parallel and then merge the reviews (args {"files": ["a.go", "b.go"]}):
{"start": "review", "nodes": {
  "review": {"for_each": "input.files", "title": "review {{ item }}", "prompt": "Review {{ item }} for bugs.",
    "schema": {"type": "object", "required": ["bugs"], "properties": {"bugs": {"type": "array", "items": {"type": "string"}}}}},
  "merge": {"prompt": "Merge these bugs into one report: {{ nodes.review.value.filter(r, r != null).map(r, r.bugs).flatten() }}"}},
 "edges": [{"from": "review", "to": "merge"}],
 "output": "nodes.merge.value.text"}

Loop example, write until the score is 8 or more, at most 3 drafts. Both nodes have max_visits 3. critique has a schema with a number "score":
"edges": [{"from": "draft", "to": "critique"},
  {"from": "critique", "to": "draft", "when": "value.score < 8.0 && nodes.draft.values.size() < 3"}]

Resume: a run lives in the memory of one coderd replica. If await_workflow says that the run is not found, for example after a restart, ` +
	`call run_flow again with the same flow (or flow_path), the same args and resume_run_id. Finished steps are reused and do not run again.`

type runFlowArgs struct {
	Flow        any    `json:"flow,omitempty" description:"The flow, a JSON object with start, nodes, edges and output. Send this or flow_path."`
	FlowPath    string `json:"flow_path,omitempty" description:"Absolute path of a JSON file in the workspace that holds the flow. Send this or flow."`
	Args        any    `json:"args,omitempty" description:"A JSON object. CEL expressions read it as input."`
	ResumeRunID string `json:"resume_run_id,omitempty" description:"Continue this run after a restart. Send the same flow (or flow_path) and args again."`
}

// flowDefinition is the run_flow manifest.
type flowDefinition struct {
	Start  string              `json:"start"`
	Nodes  map[string]flowNode `json:"nodes"`
	Edges  []flowEdge          `json:"edges"`
	Output string              `json:"output,omitempty"` // CEL; default: the value of the last node visit
}

type flowNode struct {
	Prompt    string          `json:"prompt"`               // template: text with {{ CEL }} parts
	Type      string          `json:"type,omitempty"`       // "general" (default) or "explore"
	Schema    json.RawMessage `json:"schema,omitempty"`     // as in agent(): object JSON Schema, default {text}
	Title     string          `json:"title,omitempty"`      // template; default: node ID, plus " [i]" per item and " #n" for visit n > 1
	ForEach   string          `json:"for_each,omitempty"`   // CEL list: one step per item, all started at once
	MaxVisits int             `json:"max_visits,omitempty"` // default 1
}

type flowEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	When string `json:"when,omitempty"` // CEL bool; empty means always
}

var (
	flowFields     = jsonFieldNames[flowDefinition]()
	flowNodeFields = jsonFieldNames[flowNode]()
	flowEdgeFields = jsonFieldNames[flowEdge]()

	// flowNodeIDPattern makes nodes.<id> a valid CEL select. CEL does not
	// accept the keywords in flowNodeKeywords as a field name.
	flowNodeIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	flowNodeKeywords  = []string{"false", "in", "null", "true"}
)

func jsonFieldNames[T any]() []string {
	t := reflect.TypeFor[T]()
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		names = append(names, name)
	}
	return names
}

// flowEnvSet holds the CEL environments of every flow. Each place in a
// flow sees only its own variables, so a misplaced item or value is a type
// error before the run starts.
type flowEnvSet struct {
	base *cel.Env // input and nodes: for_each, output, templates
	item *cel.Env // base plus item and index: templates of a for_each node
	edge *cel.Env // base plus value: when
}

// flowEnvs is shared by every compile. A *cel.Env is safe for concurrent
// use.
var flowEnvs = sync.OnceValues(func() (flowEnvSet, error) {
	base, err := cel.NewEnv(
		cel.Variable("input", cel.DynType),
		cel.Variable("nodes", cel.MapType(cel.StringType, cel.MapType(cel.StringType, cel.DynType))),
		ext.Strings(), ext.Lists(), ext.Math(), ext.Sets(), ext.Encoders(), ext.TwoVarComprehensions(),
		cel.OptionalTypes(),
		cel.CrossTypeNumericComparisons(true),
		cel.ParserExpressionSizeLimit(maxFlowExprSize),
	)
	if err != nil {
		return flowEnvSet{}, err
	}
	item, err := base.Extend(cel.Variable("item", cel.DynType), cel.Variable("index", cel.IntType))
	if err != nil {
		return flowEnvSet{}, err
	}
	edge, err := base.Extend(cel.Variable("value", cel.DynType))
	if err != nil {
		return flowEnvSet{}, err
	}
	return flowEnvSet{base: base, item: item, edge: edge}, nil
})

// flowProgram is a compiled flow. It walks the graph with one cursor.
type flowProgram struct {
	start  string
	nodes  map[string]*flowNodeProgram
	edges  map[string][]flowEdgeProgram // by from, in edges order
	output cel.Program                  // nil: the value of the last node visit
}

type flowNodeProgram struct {
	id        string
	typ       string
	schema    json.RawMessage
	prompt    flowTemplate
	title     flowTemplate // nil: the default title
	forEach   cel.Program  // nil: one step per visit
	maxVisits int
}

type flowEdgeProgram struct {
	index int // the position in edges, for errors
	to    string
	when  cel.Program // nil: always
}

// flowTemplate is text with {{ CEL }} parts. A part with a program is an
// expression. Any other part is text.
type flowTemplate []flowTemplatePart

type flowTemplatePart struct {
	text string // the text, or the expression source
	prg  cel.Program
}

// flowSource returns the flow JSON: args.Flow, or the content of the
// workspace file at args.FlowPath.
func flowSource(
	ctx context.Context,
	args runFlowArgs,
	getConn func(context.Context) (workspacesdk.AgentConn, error),
) ([]byte, error) {
	if path := strings.TrimSpace(args.FlowPath); path != "" {
		if args.Flow != nil {
			return nil, xerrors.New("send flow or flow_path, not both")
		}
		text, err := readWorkflowFile(ctx, "flow_path", path, getConn)
		if err != nil {
			return nil, err
		}
		return []byte(text), nil
	}
	switch flow := args.Flow.(type) {
	case nil:
		return nil, xerrors.New("send flow or flow_path")
	case string:
		// A model can send the JSON object as a string.
		return []byte(flow), nil
	default:
		raw, err := json.Marshal(flow)
		if err != nil {
			return nil, xerrors.Errorf("marshal flow: %w", err)
		}
		return raw, nil
	}
}

// compileFlow decodes src, checks it, and compiles its CEL. The error lists
// every problem, one per line, so a model can fix all of them in one retry.
func compileFlow(src []byte) (*flowProgram, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, xerrors.New("flow is required")
	}
	if len(src) > maxScriptBytes {
		return nil, xerrors.Errorf("the flow is %d bytes; at most %d bytes are allowed", len(src), maxScriptBytes)
	}
	envs, err := flowEnvs()
	if err != nil {
		return nil, xerrors.Errorf("create CEL environment: %w", err)
	}
	def, problems, err := decodeFlow(src)
	if err != nil {
		return nil, err
	}
	c := &flowCompiler{envs: envs, def: def, problems: problems}
	program := c.compile()
	if len(c.problems) > 0 {
		slices.Sort(c.problems)
		return nil, xerrors.Errorf("invalid flow:\n%s", strings.Join(slices.Compact(c.problems), "\n"))
	}
	return program, nil
}

// decodeFlow decodes src into a definition. problems names every unknown
// field and every value of the wrong JSON type. err is set only when src
// is not a JSON object.
func decodeFlow(src []byte) (def flowDefinition, problems []string, err error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(src, &top); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return def, nil, xerrors.New("the flow must be a JSON object")
		}
		return def, nil, xerrors.Errorf("the flow is not valid JSON: %w", err)
	}
	decode := func(where string, raw json.RawMessage, dst any) {
		if len(raw) == 0 {
			return
		}
		if err := json.Unmarshal(raw, dst); err != nil {
			problems = append(problems, flowTypeProblem(where, err))
		}
	}
	// object decodes one object of the flow and flags its unknown fields.
	object := func(where string, raw json.RawMessage, dst any, known []string) {
		var fields map[string]json.RawMessage
		decode(where, raw, &fields)
		problems = append(problems, unknownFlowFields(where, fields, known)...)
		decode(where, raw, dst)
	}
	problems = append(problems, unknownFlowFields("flow", top, flowFields)...)
	decode("start", top["start"], &def.Start)
	decode("output", top["output"], &def.Output)
	var nodes map[string]json.RawMessage
	decode("nodes", top["nodes"], &nodes)
	def.Nodes = make(map[string]flowNode, len(nodes))
	for id, raw := range nodes {
		var node flowNode
		object("nodes."+id, raw, &node, flowNodeFields)
		def.Nodes[id] = node
	}
	var edges []json.RawMessage
	decode("edges", top["edges"], &edges)
	def.Edges = make([]flowEdge, len(edges))
	for i, raw := range edges {
		object(fmt.Sprintf("edges[%d]", i), raw, &def.Edges[i], flowEdgeFields)
	}
	return def, problems, nil
}

// flowTypeProblem names the field of a JSON value with the wrong type.
func flowTypeProblem(where string, err error) string {
	typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err)
	if !ok {
		return fmt.Sprintf("%s: %v", where, err)
	}
	if typeErr.Field != "" {
		where += "." + typeErr.Field
	}
	want := "a string"
	switch typeErr.Type.Kind() {
	case reflect.Int:
		want = "an integer"
	case reflect.Map, reflect.Struct:
		want = "an object"
	case reflect.Slice:
		want = "a list"
	}
	return fmt.Sprintf("%s: got a JSON %s, want %s", where, typeErr.Value, want)
}

func unknownFlowFields(where string, fields map[string]json.RawMessage, known []string) []string {
	var problems []string
	for name := range fields {
		if !slices.Contains(known, name) {
			problems = append(problems, fmt.Sprintf("%s: unknown field %q", where, name))
		}
	}
	return problems
}

// flowCompiler collects the problems of one flow.
type flowCompiler struct {
	envs     flowEnvSet
	def      flowDefinition
	problems []string
}

func (c *flowCompiler) addf(format string, args ...any) {
	c.problems = append(c.problems, fmt.Sprintf(format, args...))
}

func (c *flowCompiler) compile() *flowProgram {
	def := c.def
	program := &flowProgram{
		start: def.Start,
		nodes: make(map[string]*flowNodeProgram, len(def.Nodes)),
		edges: map[string][]flowEdgeProgram{},
	}
	if len(def.Nodes) == 0 {
		c.addf("nodes: the flow needs at least one node")
	}
	ids := slices.Sorted(maps.Keys(def.Nodes))
	// Nodes without for_each run one step per visit. A for_each node can
	// start more steps; startStep enforces the run limit for those.
	visitSteps := 0
	for _, id := range ids {
		node := c.node(id, def.Nodes[id])
		program.nodes[id] = node
		if node.forEach == nil {
			visitSteps += node.maxVisits
		}
	}
	if visitSteps > maxWorkflowSteps {
		c.addf("nodes: the max_visits of the nodes without for_each add up to %d; a run has at most %d steps", visitSteps, maxWorkflowSteps)
	}
	startOK := c.nodeRef("start", def.Start)
	for i, edge := range def.Edges {
		where := fmt.Sprintf("edges[%d]", i)
		fromOK := c.nodeRef(where+".from", edge.From)
		toOK := c.nodeRef(where+".to", edge.To)
		var when cel.Program
		if strings.TrimSpace(edge.When) != "" {
			when = c.expr(c.envs.edge, where+".when", edge.When, types.BoolKind)
		}
		if fromOK && toOK {
			program.edges[edge.From] = append(program.edges[edge.From], flowEdgeProgram{index: i, to: edge.To, when: when})
		}
	}
	if startOK {
		reached := map[string]bool{def.Start: true}
		for queue := []string{def.Start}; len(queue) > 0; queue = queue[1:] {
			for _, edge := range program.edges[queue[0]] {
				if !reached[edge.to] {
					reached[edge.to] = true
					queue = append(queue, edge.to)
				}
			}
		}
		for _, id := range ids {
			if !reached[id] {
				c.addf("nodes.%s: not reachable from start %q", id, def.Start)
			}
		}
	}
	if strings.TrimSpace(def.Output) != "" {
		program.output = c.expr(c.envs.base, "output", def.Output, types.UnspecifiedKind)
	}
	return program
}

// nodeRef reports whether id names a node.
func (c *flowCompiler) nodeRef(where, id string) bool {
	switch _, ok := c.def.Nodes[id]; {
	case id == "":
		c.addf("%s: required", where)
	case !ok:
		c.addf("%s: %q is not a node", where, id)
	default:
		return true
	}
	return false
}

func (c *flowCompiler) node(id string, node flowNode) *flowNodeProgram {
	where := "nodes." + id
	if !flowNodeIDPattern.MatchString(id) || slices.Contains(flowNodeKeywords, id) {
		c.addf("%s: a node ID must match %s and must not be a CEL keyword (%s)", where, flowNodeIDPattern, strings.Join(flowNodeKeywords, ", "))
	}
	n := &flowNodeProgram{id: id, typ: node.Type, maxVisits: cmp.Or(node.MaxVisits, 1)}
	switch node.Type {
	case "", subagentTypeGeneral, subagentTypeExplore:
	default:
		c.addf("%s.type: must be %q or %q", where, subagentTypeGeneral, subagentTypeExplore)
	}
	if schema := bytes.TrimSpace(node.Schema); len(schema) > 0 && string(schema) != "null" {
		var compact bytes.Buffer
		if _, err := checkStepSchema(schema); err != nil {
			c.addf("%s.schema: %v", where, err)
		} else if err := json.Compact(&compact, schema); err == nil {
			n.schema = compact.Bytes()
		}
	}
	if node.MaxVisits < 0 || node.MaxVisits > maxWorkflowSteps {
		c.addf("%s.max_visits: must be from 1 to %d", where, maxWorkflowSteps)
	}
	env := c.envs.base
	if strings.TrimSpace(node.ForEach) != "" {
		n.forEach = c.expr(c.envs.base, where+".for_each", node.ForEach, types.ListKind)
		env = c.envs.item
	}
	if strings.TrimSpace(node.Prompt) == "" {
		c.addf("%s.prompt: required", where)
	} else {
		n.prompt = c.template(env, where+".prompt", node.Prompt)
	}
	if strings.TrimSpace(node.Title) != "" {
		n.title = c.template(env, where+".title", node.Title)
	}
	return n
}

// expr compiles one CEL expression. want is the kind of result the
// expression must have; dyn passes here and is checked at run time.
func (c *flowCompiler) expr(env *cel.Env, where, src string, want types.Kind) cel.Program {
	checked, iss := env.Compile(src)
	if iss.Err() != nil {
		for _, e := range iss.Errors() {
			msg := strings.TrimSuffix(e.Message, " (in container '')")
			if e.Location != nil && e.Location.Line() > 0 {
				c.addf("%s: %d:%d: %s", where, e.Location.Line(), e.Location.Column()+1, msg)
			} else {
				c.addf("%s: %s", where, msg)
			}
		}
		return nil
	}
	c.checkNodeRefs(where, checked)
	if out := checked.OutputType(); want != types.UnspecifiedKind && out.Kind() != want && out.Kind() != types.DynKind {
		c.addf("%s: must return %s, got %s", where, kindName(want), out)
	}
	prg, err := env.Program(checked,
		cel.CostLimit(maxFlowExprCost),
		cel.InterruptCheckFrequency(flowInterruptCheckFrequency),
	)
	if err != nil {
		c.addf("%s: %v", where, err)
		return nil
	}
	return prg
}

func kindName(k types.Kind) string {
	if k == types.ListKind {
		return "a list"
	}
	return "bool"
}

// checkNodeRefs flags nodes.<id> with an unknown ID and nodes.<id>.<field>
// with a field other than value and values.
func (c *flowCompiler) checkNodeRefs(where string, checked *cel.Ast) {
	celast.PreOrderVisit(checked.NativeRep().Expr(), celast.NewExprVisitor(func(e celast.Expr) {
		if e.Kind() != celast.SelectKind {
			return
		}
		sel := e.AsSelect()
		operand := sel.Operand()
		switch {
		case operand.Kind() == celast.IdentKind && operand.AsIdent() == "nodes":
			if _, ok := c.def.Nodes[sel.FieldName()]; !ok {
				c.addf("%s: nodes.%s is not a node", where, sel.FieldName())
			}
		case operand.Kind() == celast.SelectKind &&
			operand.AsSelect().Operand().Kind() == celast.IdentKind &&
			operand.AsSelect().Operand().AsIdent() == "nodes" &&
			sel.FieldName() != "value" && sel.FieldName() != "values":
			c.addf("%s: nodes.%s.%s: a node has only value and values", where, operand.AsSelect().FieldName(), sel.FieldName())
		}
	}))
}

func (c *flowCompiler) template(env *cel.Env, where, src string) flowTemplate {
	var parts flowTemplate
	for rest := src; ; {
		start := strings.Index(rest, "{{")
		if start < 0 {
			if rest != "" {
				parts = append(parts, flowTemplatePart{text: rest})
			}
			return parts
		}
		if start > 0 {
			parts = append(parts, flowTemplatePart{text: rest[:start]})
		}
		body := rest[start+2:]
		end := templateExprEnd(body)
		if end < 0 {
			c.addf("%s: a {{ has no closing }}", where)
			return nil
		}
		expr := strings.TrimSpace(body[:end])
		if expr == "" {
			c.addf("%s: {{ }} is empty", where)
		} else {
			parts = append(parts, flowTemplatePart{
				text: expr,
				prg:  c.expr(env, fmt.Sprintf("%s {{ %s }}", where, expr), expr, types.UnspecifiedKind),
			})
		}
		rest = body[end+2:]
	}
}

// templateExprEnd returns the index of the "}}" that closes a template
// expression. It skips string literals and balanced braces, so a map
// literal can end with "}}". It returns -1 when nothing closes it.
func templateExprEnd(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch ch := s[i]; ch {
		case '"', '\'':
			i++
			for i < len(s) && s[i] != ch {
				if s[i] == '\\' {
					i++
				}
				i++
			}
			if i >= len(s) {
				return -1
			}
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			} else if i+1 < len(s) && s[i+1] == '}' {
				return i
			}
		}
	}
	return -1
}

func (t flowTemplate) render(ctx context.Context, vars map[string]any) (string, error) {
	var b strings.Builder
	for _, part := range t {
		if part.prg == nil {
			_, _ = b.WriteString(part.text)
			continue
		}
		out, err := evalFlowExpr(ctx, part.prg, vars)
		if err != nil {
			return "", xerrors.Errorf("{{ %s }}: %w", part.text, err)
		}
		if s, ok := out.(types.String); ok {
			_, _ = b.WriteString(string(s))
			continue
		}
		raw, err := celToJSON(out)
		if err != nil {
			return "", xerrors.Errorf("{{ %s }}: %w", part.text, err)
		}
		_, _ = b.Write(raw)
	}
	return b.String(), nil
}

func evalFlowExpr(ctx context.Context, prg cel.Program, vars map[string]any) (ref.Val, error) {
	out, _, err := prg.ContextEval(ctx, vars)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return out, nil
}

// celToJSON encodes a CEL value as compact JSON. The value converts to a
// protobuf JSON value first, which handles maps and lists that
// comprehensions build. Map keys must be strings.
func celToJSON(v ref.Val) ([]byte, error) {
	native, err := v.ConvertToNative(types.JSONValueType)
	if err != nil {
		return nil, xerrors.Errorf("convert %s to JSON: %w", v.Type(), err)
	}
	pb, ok := native.(*structpb.Value)
	if !ok {
		return nil, xerrors.Errorf("convert %s to JSON: got %T", v.Type(), native)
	}
	return marshalFlowJSON(pb.AsInterface())
}

// marshalFlowJSON is json.Marshal without HTML escapes, so a prompt shows
// <, > and & as they are.
func marshalFlowJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, xerrors.Errorf("encode JSON: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// flowState is what CEL sees of a run: input and every value of every
// node.
type flowState struct {
	input  any
	values map[string][]any
}

func (s *flowState) vars() map[string]any {
	nodes := make(map[string]any, len(s.values))
	for id, values := range s.values {
		var latest any
		if len(values) > 0 {
			latest = values[len(values)-1]
		}
		nodes[id] = map[string]any{"value": latest, "values": values}
	}
	return map[string]any{"input": s.input, "nodes": nodes}
}

// run walks the graph from start and returns the output JSON. A resumed
// run walks the same path: the specs repeat, so startStep reuses the step
// chats and their values.
func (f *flowProgram) run(ctx context.Context, r *workflowRun) (json.RawMessage, error) {
	state := &flowState{values: make(map[string][]any, len(f.nodes))}
	for id := range f.nodes {
		state.values[id] = []any{}
	}
	if err := json.Unmarshal([]byte(r.argsJSON), &state.input); err != nil {
		return nil, xerrors.Errorf("decode args: %w", err)
	}
	var last any
	visits := map[string]int{}
	for cur := f.start; cur != ""; {
		node := f.nodes[cur]
		visits[cur]++
		visit := visits[cur]
		if visit > node.maxVisits {
			return nil, xerrors.Errorf("node %s ran its max_visits (%d) times", cur, node.maxVisits)
		}
		value, err := f.visit(ctx, r, node, visit, state.vars())
		if err != nil {
			return nil, err
		}
		state.values[cur] = append(state.values[cur], value)
		last = value
		next, err := f.next(ctx, cur, state.vars(), value)
		if err != nil {
			return nil, err
		}
		if next == "" {
			r.log(fmt.Sprintf("%s visit %d done, no edge matched, the flow ends", cur, visit))
		} else {
			r.log(fmt.Sprintf("%s visit %d done, next: %s", cur, visit, next))
		}
		cur = next
	}
	var raw []byte
	var err error
	if f.output == nil {
		raw, err = marshalFlowJSON(last)
	} else {
		var out ref.Val
		out, err = evalFlowExpr(ctx, f.output, state.vars())
		if err == nil {
			raw, err = celToJSON(out)
		}
	}
	if err != nil {
		return nil, xerrors.Errorf("output: %w", err)
	}
	if len(raw) > maxResultBytes {
		return nil, xerrors.Errorf("the result is %d bytes; at most %d bytes are allowed", len(raw), maxResultBytes)
	}
	return raw, nil
}

// visit runs one visit of node and returns its value.
func (*flowProgram) visit(ctx context.Context, r *workflowRun, node *flowNodeProgram, visit int, vars map[string]any) (any, error) {
	if node.forEach == nil {
		spec, err := node.spec(ctx, vars, -1, visit)
		if err != nil {
			return nil, err
		}
		step, err := r.startStep(ctx, spec)
		if err != nil {
			return nil, xerrors.Errorf("node %s: %w", node.id, err)
		}
		if err := r.waitStep(ctx, step); err != nil {
			return nil, err
		}
		raw, reason, ok := r.stepValue(step)
		if !ok {
			return nil, xerrors.Errorf("node %s failed: %s", node.id, reason)
		}
		return decodeFlowValue(node.id, raw)
	}

	out, err := evalFlowExpr(ctx, node.forEach, vars)
	if err != nil {
		return nil, xerrors.Errorf("node %s: for_each: %w", node.id, err)
	}
	items, ok := out.(traits.Lister)
	if !ok {
		return nil, xerrors.Errorf("node %s: for_each must return a list, got %s", node.id, out.Type())
	}
	size, ok := items.Size().(types.Int)
	if !ok {
		return nil, xerrors.Errorf("node %s: for_each returned a list without a size", node.id)
	}
	// Render every spec before the first step starts, so a template error
	// starts no step.
	specs := make([]stepSpec, 0, int(size))
	for i := range int(size) {
		itemVars := maps.Clone(vars)
		itemVars["item"] = items.Get(types.Int(i))
		itemVars["index"] = i
		spec, err := node.spec(ctx, itemVars, i, visit)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	steps := make([]*workflowStep, 0, len(specs))
	for _, spec := range specs {
		step, err := r.startStep(ctx, spec)
		if err != nil {
			return nil, xerrors.Errorf("node %s: %w", node.id, err)
		}
		steps = append(steps, step)
	}
	values := make([]any, 0, len(steps))
	for _, step := range steps {
		if err := r.waitStep(ctx, step); err != nil {
			return nil, err
		}
		raw, _, ok := r.stepValue(step)
		if !ok {
			values = append(values, nil)
			continue
		}
		value, err := decodeFlowValue(node.id, raw)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func decodeFlowValue(id string, raw json.RawMessage) (any, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, xerrors.Errorf("node %s: decode the step value: %w", id, err)
	}
	return value, nil
}

// spec renders the step of one visit. item is the for_each index, or -1.
func (n *flowNodeProgram) spec(ctx context.Context, vars map[string]any, item, visit int) (stepSpec, error) {
	prompt, err := n.prompt.render(ctx, vars)
	if err != nil {
		return stepSpec{}, xerrors.Errorf("node %s: prompt: %w", n.id, err)
	}
	if strings.TrimSpace(prompt) == "" {
		return stepSpec{}, xerrors.Errorf("node %s: the prompt renders to empty text", n.id)
	}
	title := n.id
	if item >= 0 {
		title += fmt.Sprintf(" [%d]", item)
	}
	if visit > 1 {
		title += fmt.Sprintf(" #%d", visit)
	}
	if n.title != nil {
		if title, err = n.title.render(ctx, vars); err != nil {
			return stepSpec{}, xerrors.Errorf("node %s: title: %w", n.id, err)
		}
	}
	return stepSpec{Prompt: prompt, Type: n.typ, Schema: n.schema, Title: title}, nil
}

// next returns the target of the first edge from from whose when is empty
// or true, or "" when none matches.
func (f *flowProgram) next(ctx context.Context, from string, vars map[string]any, value any) (string, error) {
	edgeVars := maps.Clone(vars)
	edgeVars["value"] = value
	for _, edge := range f.edges[from] {
		if edge.when == nil {
			return edge.to, nil
		}
		out, err := evalFlowExpr(ctx, edge.when, edgeVars)
		if err != nil {
			return "", xerrors.Errorf("edges[%d].when: %w", edge.index, err)
		}
		match, ok := out.(types.Bool)
		if !ok {
			return "", xerrors.Errorf("edges[%d].when must return bool, got %s", edge.index, out.Type())
		}
		if match {
			return edge.to, nil
		}
	}
	return "", nil
}
