package echo

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	protobuf "google.golang.org/protobuf/proto"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

// ProvisionApplyWithAgent returns provision responses that will mock a fake
// "aws_instance" resource with an agent that has the given auth token.
func ProvisionApplyWithAgent(authToken string) []*proto.Response {
	return []*proto.Response{{
		Type: &proto.Response_Apply{
			Apply: &proto.ApplyComplete{
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
			},
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
			filepath.Join(sess.WorkDirectory, fmt.Sprintf("%d.%s.%s", i, trans, suffix)),
			filepath.Join(sess.WorkDirectory, fmt.Sprintf("%d.%s", i, suffix)),
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

	// ProvisionApply and ProvisionPlan are used to mock ALL responses of
	// Apply and Plan, regardless of transition.
	ProvisionApply []*proto.Response
	ProvisionPlan  []*proto.Response

	// ProvisionApplyMap and ProvisionPlanMap are used to mock specific
	// transition responses. They are prioritized over the generic responses.
	ProvisionApplyMap map[proto.WorkspaceTransition][]*proto.Response
	ProvisionPlanMap  map[proto.WorkspaceTransition][]*proto.Response
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
			ParseComplete, ApplyComplete, PlanComplete,
			nil, nil,
		}
	}
	if responses.ProvisionPlan == nil {
		for _, resp := range responses.ProvisionApply {
			if resp.GetLog() != nil {
				responses.ProvisionPlan = append(responses.ProvisionPlan, resp)
				continue
			}
			responses.ProvisionPlan = append(responses.ProvisionPlan, &proto.Response{
				Type: &proto.Response_Plan{Plan: &proto.PlanComplete{
					Error:                 resp.GetApply().GetError(),
					Resources:             resp.GetApply().GetResources(),
					Parameters:            resp.GetApply().GetParameters(),
					ExternalAuthProviders: resp.GetApply().GetExternalAuthProviders(),
					Plan:                  []byte("{}"),
				}},
			})
		}
	}

	for _, resp := range responses.ProvisionPlan {
		plan := resp.GetPlan()
		if plan == nil {
			continue
		}

		if len(plan.Plan) == 0 {
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
		logger.Debug(context.Background(), "proto written", slog.F("name", name), slog.F("bytes_written", n))

		return nil
	}
	for index, response := range responses.Parse {
		err := writeProto(fmt.Sprintf("%d.parse.protobuf", index), response)
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
	for trans, m := range responses.ProvisionApplyMap {
		for i, rs := range m {
			err := writeProto(fmt.Sprintf("%d.%s.apply.protobuf", i, strings.ToLower(trans.String())), rs)
			if err != nil {
				return nil, err
			}
		}
	}
	for trans, m := range responses.ProvisionPlanMap {
		for i, rs := range m {
			err := writeProto(fmt.Sprintf("%d.%s.plan.protobuf", i, strings.ToLower(trans.String())), rs)
			if err != nil {
				return nil, err
			}
		}
	}
	// `writer.Close()` function flushes the writer buffer, and adds extra padding to create a legal tarball.
	err := writer.Close()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func WithResources(resources []*proto.Resource) *Responses {
	return &Responses{
		Parse: ParseComplete,
		ProvisionApply: []*proto.Response{{Type: &proto.Response_Apply{Apply: &proto.ApplyComplete{
			Resources: resources,
		}}}},
		ProvisionPlan: []*proto.Response{{Type: &proto.Response_Plan{Plan: &proto.PlanComplete{
			Resources: resources,
			Plan:      []byte("{}"),
		}}}},
	}
}
