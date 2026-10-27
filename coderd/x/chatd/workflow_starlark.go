package chatd

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	starlarkjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"golang.org/x/xerrors"
)

const workflowFileName = "workflow.star"

// workflowFileOptions allow while loops. Recursion stays off, and load()
// fails because the thread has no Load function.
var workflowFileOptions = &syntax.FileOptions{While: true}

var workflowBuiltinNames = []string{"agent", "spawn", "wait", "log", "json"}

// workflowProgram is a compiled script. The host API, Universe and the
// json module are the only names a script can reach. Universe has no
// clock, no random source, no files and no network.
type workflowProgram struct {
	program *starlark.Program
}

func compileWorkflow(script string) (*workflowProgram, error) {
	file, program, err := starlark.SourceProgramOptions(workflowFileOptions, workflowFileName, script,
		func(name string) bool { return slices.Contains(workflowBuiltinNames, name) })
	if err != nil {
		return nil, xerrors.Errorf("compile script: %w", err)
	}
	hasMain := slices.ContainsFunc(file.Stmts, func(stmt syntax.Stmt) bool {
		def, ok := stmt.(*syntax.DefStmt)
		return ok && def.Name.Name == "main"
	})
	if !hasMain {
		return nil, xerrors.New("the script must define main(args)")
	}
	return &workflowProgram{program: program}, nil
}

// run executes the script and calls main(args). It returns the JSON that
// main returned.
func (w *workflowProgram) run(ctx context.Context, r *workflowRun) (json.RawMessage, error) {
	thread := &starlark.Thread{
		Name:  "workflow",
		Print: func(_ *starlark.Thread, msg string) { r.log(msg) },
	}
	thread.SetMaxExecutionSteps(maxScriptSteps)
	stop := context.AfterFunc(ctx, func() { thread.Cancel("workflow stopped") })
	defer stop()

	host := &workflowHost{ctx: ctx, run: r}
	globals, err := w.program.Init(thread, host.predeclared())
	if err != nil {
		return nil, scriptError(err)
	}
	mainFn, ok := globals["main"].(starlark.Callable)
	if !ok {
		return nil, xerrors.New("the script must define main(args)")
	}
	args, err := jsonDecode(thread, r.argsJSON)
	if err != nil {
		return nil, xerrors.Errorf("decode args: %w", err)
	}
	out, err := starlark.Call(thread, mainFn, starlark.Tuple{args}, nil)
	if err != nil {
		return nil, scriptError(err)
	}
	raw, err := jsonEncode(thread, out)
	if err != nil {
		return nil, xerrors.Errorf("main(args) must return a JSON value: %w", err)
	}
	if len(raw) > maxResultBytes {
		return nil, xerrors.Errorf("the result is %d bytes; at most %d bytes are allowed", len(raw), maxResultBytes)
	}
	return json.RawMessage(raw), nil
}

// scriptError keeps the Starlark backtrace, which names the line.
func scriptError(err error) error {
	if evalErr, ok := err.(*starlark.EvalError); ok { //nolint:errorlint // starlark returns it unwrapped
		return xerrors.New(evalErr.Backtrace())
	}
	return err
}

func jsonDecode(thread *starlark.Thread, text string) (starlark.Value, error) {
	return starlark.Call(thread, starlarkjson.Module.Members["decode"], starlark.Tuple{starlark.String(text)}, nil)
}

func jsonEncode(thread *starlark.Thread, value starlark.Value) (string, error) {
	out, err := starlark.Call(thread, starlarkjson.Module.Members["encode"], starlark.Tuple{value}, nil)
	if err != nil {
		return "", err
	}
	s, ok := out.(starlark.String)
	if !ok {
		return "", xerrors.Errorf("json.encode returned %s", out.Type())
	}
	return string(s), nil
}

// workflowHandle is the value that spawn() returns.
type workflowHandle struct {
	step *workflowStep
}

var _ starlark.Value = (*workflowHandle)(nil)

func (h *workflowHandle) String() string      { return fmt.Sprintf("<step %s>", h.step.chatID) }
func (*workflowHandle) Type() string          { return "step" }
func (*workflowHandle) Freeze()               {}
func (*workflowHandle) Truth() starlark.Bool  { return starlark.True }
func (*workflowHandle) Hash() (uint32, error) { return 0, xerrors.New("unhashable type: step") }

// workflowHost holds the built-ins of one run.
type workflowHost struct {
	ctx context.Context
	run *workflowRun
}

func (h *workflowHost) predeclared() starlark.StringDict {
	return starlark.StringDict{
		"agent": starlark.NewBuiltin("agent", h.agent),
		"spawn": starlark.NewBuiltin("spawn", h.spawn),
		"wait":  starlark.NewBuiltin("wait", h.wait),
		"log":   starlark.NewBuiltin("log", h.log),
		"json":  starlarkjson.Module,
	}
}

func (*workflowHost) stepSpec(
	thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple,
) (stepSpec, error) {
	var (
		prompt, typ, title string
		schema             starlark.Value = starlark.None
	)
	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"prompt", &prompt, "type?", &typ, "schema?", &schema, "title?", &title); err != nil {
		return stepSpec{}, err
	}
	spec := stepSpec{Prompt: prompt, Type: strings.TrimSpace(typ), Title: title}
	if schema != starlark.None {
		raw, err := jsonEncode(thread, schema)
		if err != nil {
			return stepSpec{}, xerrors.Errorf("%s: schema: %w", b.Name(), err)
		}
		spec.Schema = json.RawMessage(raw)
	}
	return spec, nil
}

func (h *workflowHost) agent(
	thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple,
) (starlark.Value, error) {
	spec, err := h.stepSpec(thread, b, args, kwargs)
	if err != nil {
		return nil, err
	}
	step, err := h.run.startStep(h.ctx, spec)
	if err != nil {
		return nil, err
	}
	if err := h.run.waitStep(h.ctx, step); err != nil {
		return nil, err
	}
	value, reason, ok := h.run.stepValue(step)
	if !ok {
		return nil, xerrors.Errorf("step %q failed: %s", step.title, reason)
	}
	return jsonDecode(thread, string(value))
}

func (h *workflowHost) spawn(
	thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple,
) (starlark.Value, error) {
	spec, err := h.stepSpec(thread, b, args, kwargs)
	if err != nil {
		return nil, err
	}
	step, err := h.run.startStep(h.ctx, spec)
	if err != nil {
		return nil, err
	}
	return &workflowHandle{step: step}, nil
}

// wait takes one handle or an iterable of handles. Steps run at the same
// time in chatd, so one wait after the other takes as long as the slowest
// step.
func (h *workflowHost) wait(
	thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var target starlark.Value
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &target); err != nil {
		return nil, err
	}
	if handle, ok := target.(*workflowHandle); ok {
		return h.waitOne(thread, handle)
	}
	iterable, ok := target.(starlark.Iterable)
	if !ok {
		return nil, xerrors.Errorf("wait: got %s, want a step handle or a list of handles", target.Type())
	}
	var handles []*workflowHandle
	iter := iterable.Iterate()
	defer iter.Done()
	var x starlark.Value
	for iter.Next(&x) {
		handle, ok := x.(*workflowHandle)
		if !ok {
			return nil, xerrors.Errorf("wait: element %d is %s, want a step handle", len(handles), x.Type())
		}
		handles = append(handles, handle)
	}
	values := make([]starlark.Value, 0, len(handles))
	for _, handle := range handles {
		value, err := h.waitOne(thread, handle)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return starlark.NewList(values), nil
}

// waitOne gives None for a failed step.
func (h *workflowHost) waitOne(thread *starlark.Thread, handle *workflowHandle) (starlark.Value, error) {
	if err := h.run.waitStep(h.ctx, handle.step); err != nil {
		return nil, err
	}
	value, _, ok := h.run.stepValue(handle.step)
	if !ok {
		return starlark.None, nil
	}
	return jsonDecode(thread, string(value))
}

func (h *workflowHost) log(
	_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple,
) (starlark.Value, error) {
	if len(kwargs) > 0 {
		return nil, xerrors.Errorf("%s: unexpected keyword arguments", b.Name())
	}
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		if s, ok := arg.(starlark.String); ok {
			parts = append(parts, string(s))
			continue
		}
		parts = append(parts, arg.String())
	}
	h.run.log(strings.Join(parts, " "))
	return starlark.None, nil
}
