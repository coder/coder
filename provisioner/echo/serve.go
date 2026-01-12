package echo

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"
	protobuf "google.golang.org/protobuf/proto"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

// ProvisionGraphWithAgentAndAPIKeyScope returns provision responses that will mock a fake
// "aws_instance" resource with an agent that has the given auth token.
func ProvisionGraphWithAgentAndAPIKeyScope(authToken string, apiKeyScope string) []*proto.Response {
	return []*proto.Response{{
		Type: &proto.Response_Graph{
			Graph: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name: "example_with_scope",
					Type: "aws_instance",
					Agents: []*proto.Agent{{
						Id:   uuid.NewString(),
						Name: "example",
						Auth: &proto.Agent_Token{
							Token: authToken,
						},
						ApiKeyScope: apiKeyScope,
					}},
				}},
			},
		},
	}}
}

// ProvisionGraphWithAgent returns provision responses that will mock a fake
// "aws_instance" resource with an agent that has the given auth token.
func ProvisionGraphWithAgent(authToken string, muts ...func(g *proto.GraphComplete)) []*proto.Response {
	gc := &proto.GraphComplete{
		Resources: []*proto.Resource{{
			Name: "example",
			Type: "aws_instance",
			Agents: []*proto.Agent{{
				Id:   uuid.NewString(),
				Name: "example",
				Auth: &proto.Agent_Token{
					Token: authToken,
				},
			}},
		}},
	}
	for _, mut := range muts {
		mut(gc)
	}

	return []*proto.Response{{
		Type: &proto.Response_Graph{
			Graph: gc,
		},
	}}
}

var (
	// ParseComplete is a helper to indicate an empty parse completion.
	ParseComplete = []*proto.Response{{
		Type: &proto.Response_Parse{
			Parse: &proto.ParseComplete{},
		},
	}}
	// InitComplete is a helper to indicate an empty init completion.
	InitComplete = []*proto.Response{{
		Type: &proto.Response_Init{
			Init: &proto.InitComplete{
				ModuleFiles: []byte{},
			},
		},
	}}
	// PlanComplete is a helper to indicate an empty provision completion.
	PlanComplete = []*proto.Response{{
		Type: &proto.Response_Plan{
			Plan: &proto.PlanComplete{
				Plan: []byte("{}"),
			},
		},
	}}
	// ApplyComplete is a helper to indicate an empty provision completion.
	ApplyComplete = []*proto.Response{{
		Type: &proto.Response_Apply{
			Apply: &proto.ApplyComplete{},
		},
	}}
	GraphComplete = []*proto.Response{{
		Type: &proto.Response_Graph{
			Graph: &proto.GraphComplete{},
		},
	}}

	InitFailed = []*proto.Response{{
		Type: &proto.Response_Init{
			Init: &proto.InitComplete{
				Error: "failed!",
			},
		},
	}}
	// PlanFailed is a helper to convey a failed plan operation
	PlanFailed = []*proto.Response{{
		Type: &proto.Response_Plan{
			Plan: &proto.PlanComplete{
				Error: "failed!",
			},
		},
	}}
	// ApplyFailed is a helper to convey a failed apply operation
	ApplyFailed = []*proto.Response{{
		Type: &proto.Response_Apply{
			Apply: &proto.ApplyComplete{
				Error: "failed!",
			},
		},
	}}
	GraphFailed = []*proto.Response{{
		Type: &proto.Response_Graph{
			Graph: &proto.GraphComplete{
				Error: "failed!",
			},
		},
	}}
)

// Serve starts the echo provisioner.
func Serve(ctx context.Context, options *provisionersdk.ServeOptions) error {
	return provisionersdk.Serve(ctx, &echo{}, options)
}

// The echo provisioner serves as a dummy provisioner primarily
// used for testing. It echos responses from JSON files in the
// format %d.protobuf. It's used for testing.
type echo struct{}

func readResponses(sess *provisionersdk.Session, trans string, suffix string) ([]*proto.Response, error) {
	var responses []*proto.Response
	for i := 0; ; i++ {
		paths := []string{
			// Try more specific path first, then fallback to generic.
			filepath.Join(sess.Files.WorkDirectory(), fmt.Sprintf("%d.%s.%s", i, trans, suffix)),
			filepath.Join(sess.Files.WorkDirectory(), fmt.Sprintf("%d.%s", i, suffix)),
		}
		for pathIndex, path := range paths {
			_, err := os.Stat(path)
			if err != nil && pathIndex == (len(paths)-1) {
				// If there are zero messages, something is wrong
				if i == 0 {
					// Error if nothing is around to enable failed states.
					return nil, xerrors.Errorf("no state: %w", err)
				}
				// Otherwise, we've read all responses
				return responses, nil
			}
			if err != nil {
				// try next path
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, xerrors.Errorf("read file %q: %w", path, err)
			}
			response := new(proto.Response)
			err = protobuf.Unmarshal(data, response)
			if err != nil {
				return nil, xerrors.Errorf("unmarshal: %w", err)
			}
			responses = append(responses, response)
			break
		}
	}
}

// Parse reads requests from the provided directory to stream responses.
func (*echo) Parse(sess *provisionersdk.Session, _ *proto.ParseRequest, _ <-chan struct{}) *proto.ParseComplete {
	responses, err := readResponses(sess, "unspecified", "parse.protobuf")
	if err != nil {
		return &proto.ParseComplete{Error: err.Error()}
	}
	for _, response := range responses {
		if log := response.GetLog(); log != nil {
			sess.ProvisionLog(log.Level, log.Output)
		}
		if complete := response.GetParse(); complete != nil {
			return complete
		}
	}

	// if we didn't get a complete from the filesystem, that's an error
	return provisionersdk.ParseErrorf("complete response missing")
}

func (*echo) Init(sess *provisionersdk.Session, req *provisionersdk.InitRequest, canceledOrComplete <-chan struct{}) *proto.InitComplete {
	err := sess.Files.ExtractArchive(sess.Context(), sess.Logger, afero.NewOsFs(), req.TemplateSourceArchive, nil)
	if err != nil {
		return provisionersdk.InitErrorf("extract archive: %s", err.Error())
	}

	responses, err := readResponses(
		sess,
		"", // transition not supported for init graph responses
		"init.protobuf")
	if err != nil {
		return &proto.InitComplete{Error: err.Error()}
	}
	for _, response := range responses {
		if log := response.GetLog(); log != nil {
			sess.ProvisionLog(log.Level, log.Output)
		}
		if complete := response.GetInit(); complete != nil {
			return complete
		}
	}

	// some tests use Echo without a complete response to test cancel
	<-canceledOrComplete
	return provisionersdk.InitErrorf("canceled")
}

func (*echo) Graph(sess *provisionersdk.Session, req *proto.GraphRequest, canceledOrComplete <-chan struct{}) *proto.GraphComplete {
	responses, err := readResponses(
		sess,
		strings.ToLower(req.GetMetadata().GetWorkspaceTransition().String()),
		"graph.protobuf")
	if err != nil {
		return &proto.GraphComplete{Error: err.Error()}
	}
	for _, response := range responses {
		if log := response.GetLog(); log != nil {
			sess.ProvisionLog(log.Level, log.Output)
		}
		if complete := response.GetGraph(); complete != nil {
			if len(complete.AiTasks) > 0 {
				// These two fields are linked; if there are AI tasks, indicate that.
				complete.HasAiTasks = true
			}
			return complete
		}
	}

	// some tests use Echo without a complete response to test cancel
	<-canceledOrComplete
	return provisionersdk.GraphError("canceled")
}

// Plan reads requests from the provided directory to stream responses.
func (*echo) Plan(sess *provisionersdk.Session, req *proto.PlanRequest, canceledOrComplete <-chan struct{}) *proto.PlanComplete {
	responses, err := readResponses(
		sess,
		strings.ToLower(req.GetMetadata().GetWorkspaceTransition().String()),
		"plan.protobuf")
	if err != nil {
		return &proto.PlanComplete{Error: err.Error()}
	}
	for _, response := range responses {
		if log := response.GetLog(); log != nil {
			sess.ProvisionLog(log.Level, log.Output)
		}
		if complete := response.GetPlan(); complete != nil {
			return complete
		}
	}

	// some tests use Echo without a complete response to test cancel
	<-canceledOrComplete
	return provisionersdk.PlanErrorf("canceled")
}

// Apply reads requests from the provided directory to stream responses.
func (*echo) Apply(sess *provisionersdk.Session, req *proto.ApplyRequest, canceledOrComplete <-chan struct{}) *proto.ApplyComplete {
	responses, err := readResponses(
		sess,
		strings.ToLower(req.GetMetadata().GetWorkspaceTransition().String()),
		"apply.protobuf")
	if err != nil {
		return &proto.ApplyComplete{Error: err.Error()}
	}
	for _, response := range responses {
		if log := response.GetLog(); log != nil {
			sess.ProvisionLog(log.Level, log.Output)
		}
		if complete := response.GetApply(); complete != nil {
			return complete
		}
	}

	// some tests use Echo without a complete response to test cancel
	<-canceledOrComplete
	return provisionersdk.ApplyErrorf("canceled")
}

func (*echo) Shutdown(_ context.Context, _ *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

// Responses is a collection of mocked responses to Provision operations.
type Responses struct {
	Parse []*proto.Response

	// Used to mock ALL responses regardless of transition.
	ProvisionInit  []*proto.Response
	ProvisionPlan  []*proto.Response
	ProvisionApply []*proto.Response
	ProvisionGraph []*proto.Response

	// Used to mock specific transition responses. They are prioritized over the generic responses.
	ProvisionPlanMap  map[proto.WorkspaceTransition][]*proto.Response
	ProvisionApplyMap map[proto.WorkspaceTransition][]*proto.Response
	ProvisionGraphMap map[proto.WorkspaceTransition][]*proto.Response

	ExtraFiles map[string][]byte
}

func isType[T any](x any) bool {
	_, ok := x.(T)
	return ok
}

func (r *Responses) Valid() error {
	isLog := isType[*proto.Response_Log]
	isParse := isType[*proto.Response_Parse]
	isInit := isType[*proto.Response_Init]
	isDataUpload := isType[*proto.Response_DataUpload]
	isChunkPiece := isType[*proto.Response_ChunkPiece]
	isPlan := isType[*proto.Response_Plan]
	isApply := isType[*proto.Response_Apply]
	isGraph := isType[*proto.Response_Graph]

	for _, parse := range r.Parse {
		ty := parse.Type
		if !(isParse(ty) || isLog(ty)) {
			return xerrors.Errorf("invalid parse response type: %T", ty)
		}
	}

	for _, init := range r.ProvisionInit {
		ty := init.Type
		if !(isInit(ty) || isLog(ty) || isChunkPiece(ty) || isDataUpload(ty)) {
			return xerrors.Errorf("invalid init response type: %T", ty)
		}
	}

	for _, plan := range r.ProvisionPlan {
		ty := plan.Type
		if !(isPlan(ty) || isLog(ty)) {
			return xerrors.Errorf("invalid plan response type: %T", ty)
		}
	}

	for _, apply := range r.ProvisionApply {
		ty := apply.Type
		if !(isApply(ty) || isLog(ty)) {
			return xerrors.Errorf("invalid apply response type: %T", ty)
		}
	}

	for _, graph := range r.ProvisionGraph {
		ty := graph.Type
		if !(isGraph(ty) || isLog(ty)) {
			return xerrors.Errorf("invalid graph response type: %T", ty)
		}
	}

	return nil
}

// Tar returns a tar archive of responses to provisioner operations.
func Tar(responses *Responses) ([]byte, error) {
	logger := slog.Make()
	return TarWithOptions(context.Background(), logger, responses)
}

// TarWithOptions returns a tar archive of responses to provisioner operations,
// but it gives more insight into the archiving process.
func TarWithOptions(ctx context.Context, logger slog.Logger, responses *Responses) ([]byte, error) {
	logger = logger.Named("echo_tar")

	if responses == nil {
		responses = &Responses{
			Parse:             ParseComplete,
			ProvisionInit:     InitComplete,
			ProvisionPlan:     PlanComplete,
			ProvisionApply:    ApplyComplete,
			ProvisionGraph:    GraphComplete,
			ProvisionApplyMap: nil,
			ProvisionPlanMap:  nil,
			ExtraFiles:        nil,
		}
	}

	// Apply sane defaults for missing responses.
	if responses.Parse == nil {
		responses.Parse = ParseComplete
	}
	if responses.ProvisionInit == nil {
		responses.ProvisionInit = InitComplete
	}
	if responses.ProvisionPlan == nil {
		responses.ProvisionPlan = PlanComplete

		// If a graph response exists, make sure it matches the plan.
		for _, resp := range responses.ProvisionGraph {
			if resp.GetLog() != nil {
				continue
			}
			if g := resp.GetGraph(); g != nil {
				dailycost := int32(0)
				for _, r := range g.GetResources() {
					dailycost += r.DailyCost
				}
				responses.ProvisionPlan = []*proto.Response{{
					Type: &proto.Response_Plan{
						Plan: &proto.PlanComplete{
							Plan: []byte("{}"),
							//nolint:gosec // the number of resources will not exceed int32
							AiTaskCount: int32(len(g.GetAiTasks())),
							DailyCost:   dailycost,
						},
					},
				}}
				break
			}
		}
	}
	if responses.ProvisionApply == nil {
		responses.ProvisionApply = ApplyComplete
	}
	if responses.ProvisionGraph == nil {
		responses.ProvisionGraph = GraphComplete
	}

	for _, resp := range responses.ProvisionPlan {
		plan := resp.GetPlan()
		if plan == nil {
			continue
		}

		if plan.Error == "" && len(plan.Plan) == 0 {
			plan.Plan = []byte("{}")
		}
	}

	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)

	writeProto := func(name string, message protobuf.Message) error {
		data, err := protobuf.Marshal(message)
		if err != nil {
			return err
		}
		logger.Debug(ctx, "write proto", slog.F("name", name), slog.F("message", string(data)))

		err = writer.WriteHeader(&tar.Header{
			Name: name,
			Size: int64(len(data)),
			Mode: 0o644,
		})
		if err != nil {
			return err
		}

		n, err := writer.Write(data)
		if err != nil {
			return err
		}

		response := new(proto.Response)
		err = protobuf.Unmarshal(data, response)
		if err != nil {
			return xerrors.Errorf("you must have saved the wrong type, the proto cannot unmarshal: %w", err)
		}

		logger.Debug(context.Background(), "proto written", slog.F("name", name), slog.F("bytes_written", n))

		return nil
	}
	for index, response := range responses.Parse {
		err := writeProto(fmt.Sprintf("%d.parse.protobuf", index), response)
		if err != nil {
			return nil, err
		}
	}
	for index, response := range responses.ProvisionInit {
		err := writeProto(fmt.Sprintf("%d.init.protobuf", index), response)
		if err != nil {
			return nil, err
		}
	}
	for index, response := range responses.ProvisionApply {
		err := writeProto(fmt.Sprintf("%d.apply.protobuf", index), response)
		if err != nil {
			return nil, err
		}
	}
	for index, response := range responses.ProvisionPlan {
		err := writeProto(fmt.Sprintf("%d.plan.protobuf", index), response)
		if err != nil {
			return nil, err
		}
	}
	for index, response := range responses.ProvisionGraph {
		err := writeProto(fmt.Sprintf("%d.graph.protobuf", index), response)
		if err != nil {
			return nil, err
		}
	}
	for trans, m := range responses.ProvisionApplyMap {
		for i, rs := range m {
			err := writeProto(fmt.Sprintf("%d.%s.apply.protobuf", i, strings.ToLower(trans.String())), rs)
			if err != nil {
				return nil, err
			}
		}
	}
	for trans, m := range responses.ProvisionPlanMap {
		for i, resp := range m {
			plan := resp.GetPlan()
			if plan != nil {
				if plan.Error == "" && len(plan.Plan) == 0 {
					plan.Plan = []byte("{}")
				}
			}

			err := writeProto(fmt.Sprintf("%d.%s.plan.protobuf", i, strings.ToLower(trans.String())), resp)
			if err != nil {
				return nil, err
			}
		}
	}
	for trans, m := range responses.ProvisionGraphMap {
		for i, resp := range m {
			err := writeProto(fmt.Sprintf("%d.%s.graph.protobuf", i, strings.ToLower(trans.String())), resp)
			if err != nil {
				return nil, err
			}
		}
	}
	dirs := []string{}
	for name, content := range responses.ExtraFiles {
		logger.Debug(ctx, "extra file", slog.F("name", name))

		// We need to add directories before any files that use them. But, we only need to do this
		// once.
		dir := filepath.Dir(name)
		if dir != "." && !slices.Contains(dirs, dir) {
			logger.Debug(ctx, "adding extra file directory", slog.F("dir", dir))
			dirs = append(dirs, dir)
			err := writer.WriteHeader(&tar.Header{
				Name:     dir,
				Mode:     0o755,
				Typeflag: tar.TypeDir,
			})
			if err != nil {
				return nil, err
			}
		}

		err := writer.WriteHeader(&tar.Header{
			Name: name,
			Size: int64(len(content)),
			Mode: 0o644,
		})
		if err != nil {
			return nil, err
		}

		n, err := writer.Write(content)
		if err != nil {
			return nil, err
		}

		logger.Debug(context.Background(), "extra file written", slog.F("name", name), slog.F("bytes_written", n))
	}

	// Write a main.tf with the appropriate parameters. This is to write terraform
	// that matches the parameters defined in the responses. Dynamic parameters
	// parsed these, even in the echo provisioner.
	var mainTF bytes.Buffer
	for _, respPlan := range responses.ProvisionGraph {
		plan := respPlan.GetGraph()
		if plan == nil {
			continue
		}

		for _, param := range plan.Parameters {
			paramTF, err := ParameterTerraform(param)
			if err != nil {
				return nil, xerrors.Errorf("parameter terraform: %w", err)
			}
			_, _ = mainTF.WriteString(paramTF)
		}
	}

	if mainTF.Len() > 0 {
		mainTFData := `
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}
` + mainTF.String()

		_ = writer.WriteHeader(&tar.Header{
			Name: `main.tf`,
			Size: int64(len(mainTFData)),
			Mode: 0o644,
		})
		_, _ = writer.Write([]byte(mainTFData))
	}

	// `writer.Close()` function flushes the writer buffer, and adds extra padding to create a legal tarball.
	err := writer.Close()
	if err != nil {
		return nil, err
	}

	if err := responses.Valid(); err != nil {
		return nil, xerrors.Errorf("responses invalid: %w", err)
	}

	return buffer.Bytes(), nil
}

// ParameterTerraform will create a Terraform data block for the provided parameter.
func ParameterTerraform(param *proto.RichParameter) (string, error) {
	tmpl := template.Must(template.New("parameter").Funcs(map[string]any{
		"showValidation": func(v *proto.RichParameter) bool {
			return v != nil && (v.ValidationMax != nil || v.ValidationMin != nil ||
				v.ValidationError != "" || v.ValidationRegex != "" ||
				v.ValidationMonotonic != "")
		},
		"formType": func(v *proto.RichParameter) string {
			s, _ := proto.ProviderFormType(v.FormType)
			return string(s)
		},
	}).Parse(`
data "coder_parameter" "{{ .Name }}" {
  name         = "{{ .Name }}"
  display_name = "{{ .DisplayName }}"
  description  = "{{ .Description }}"
  icon  = "{{ .Icon }}"
  mutable      = {{ .Mutable }}
  ephemeral    = {{ .Ephemeral }}
  order 	 = {{ .Order }}
{{- if .DefaultValue }}
  default      = {{ .DefaultValue }}
{{- end }}
{{- if .Type }}
  type      = "{{ .Type }}"
{{- end }}
{{- if .FormType }}
  form_type      = "{{ formType . }}"
{{- end }}
{{- range .Options }}
  option {
    name  = "{{ .Name }}"
    value = "{{ .Value }}"
  }
{{- end }}
{{- if showValidation .}}
  validation {
	{{- if .ValidationRegex }}
	regex = "{{ .ValidationRegex }}"
	{{- end }}
	{{- if .ValidationError }}
	error = "{{ .ValidationError }}"
	{{- end }}
	{{- if .ValidationMin }}
	min   = {{ .ValidationMin }}
	{{- end }}
	{{- if .ValidationMax }}
	max   = {{ .ValidationMax }}
	{{- end }}
	{{- if .ValidationMonotonic }}
	monotonic = "{{ .ValidationMonotonic }}"
	{{- end }}
  }
{{- end }}
}
`))

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, param)
	return buf.String(), err
}

func WithResources(resources []*proto.Resource) *Responses {
	return &Responses{
		Parse:          ParseComplete,
		ProvisionInit:  InitComplete,
		ProvisionApply: []*proto.Response{{Type: &proto.Response_Apply{Apply: &proto.ApplyComplete{}}}},
		ProvisionGraph: []*proto.Response{{Type: &proto.Response_Graph{Graph: &proto.GraphComplete{
			Resources: resources,
		}}}},
		ProvisionPlan: []*proto.Response{{Type: &proto.Response_Plan{Plan: &proto.PlanComplete{
			Plan: []byte("{}"),
		}}}},
	}
}

func WithExtraFiles(extraFiles map[string][]byte) *Responses {
	return &Responses{
		Parse:          ParseComplete,
		ProvisionInit:  InitComplete,
		ProvisionApply: ApplyComplete,
		ProvisionPlan:  PlanComplete,
		ProvisionGraph: GraphComplete,
		ExtraFiles:     extraFiles,
	}
}
