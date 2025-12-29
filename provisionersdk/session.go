package provisionersdk

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"
	protobuf "google.golang.org/protobuf/proto"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
	"github.com/coder/coder/v2/provisionersdk/tfpath/x"
)

// protoServer is a wrapper that translates the dRPC protocol into a Session with method calls into the Server.
type protoServer struct {
	server Server
	opts   ServeOptions
}

func (p *protoServer) Session(stream proto.DRPCProvisioner_SessionStream) error {
	sessID := uuid.New().String()
	s := &Session{
		Logger: p.opts.Logger.With(slog.F("session_id", sessID)),
		stream: stream,
		server: p.server,
	}

	s.Files = tfpath.Session(p.opts.WorkDirectory, sessID)

	defer func() {
		s.Files.Cleanup(s.Context(), s.Logger, afero.NewOsFs())
	}()

	req, err := stream.Recv()
	if err != nil {
		return xerrors.Errorf("receive config: %w", err)
	}
	config := req.GetConfig()
	if config == nil {
		return xerrors.New("first request must be Config")
	}
	s.Config = config
	if s.Config.ProvisionerLogLevel != "" {
		s.logLevel = proto.LogLevel_value[strings.ToUpper(s.Config.ProvisionerLogLevel)]
	}

	// Cleanup any previously left stale sessions.
	err = s.Files.CleanStaleSessions(s.Context(), s.Logger, afero.NewOsFs(), time.Now())
	if err != nil {
		return xerrors.Errorf("unable to clean stale sessions %q: %w", s.Files, err)
	}

	return s.handleRequests()
}

func (s *Session) requestReader(done <-chan struct{}) <-chan *proto.Request {
	ch := make(chan *proto.Request)
	go func() {
		defer close(ch)
		for {
			req, err := s.stream.Recv()
			if err != nil {
				if !xerrors.Is(err, io.EOF) {
					s.Logger.Warn(s.Context(), "recv done on Session", slog.Error(err))
				} else {
					s.Logger.Info(s.Context(), "recv done on Session")
				}
				return
			}
			select {
			case ch <- req:
				continue
			case <-done:
				return
			}
		}
	}()
	return ch
}

func (s *Session) handleRequests() error {
	done := make(chan struct{})
	defer close(done)
	requests := s.requestReader(done)
	planned := false
	for req := range requests {
		if req.GetCancel() != nil {
			s.Logger.Warn(s.Context(), "ignoring cancel before request or after complete")
			continue
		}
		resp := &proto.Response{}
		if parse := req.GetParse(); parse != nil {
			if !s.initialized {
				// Files must be initialized before parsing.
				return xerrors.New("cannot parse before successful init")
			}
			r := &request[*proto.ParseRequest, *proto.ParseComplete]{
				req:      parse,
				session:  s,
				serverFn: s.server.Parse,
				cancels:  requests,
			}
			complete, err := r.do()
			if err != nil {
				return err
			}
			// Handle README centrally, so that individual provisioners don't need to mess with it.
			readme, err := os.ReadFile(s.Files.ReadmeFilePath())
			if err == nil {
				complete.Readme = readme
			} else {
				s.Logger.Debug(s.Context(), "failed to parse readme (missing ok)", slog.Error(err))
			}
			resp.Type = &proto.Response_Parse{Parse: complete}
		}
		if init := req.GetInit(); init != nil {
			if s.initialized {
				return xerrors.New("cannot init more than once per session")
			}
			initResp, err := s.handleInitRequest(init, requests)
			if err != nil {
				return err
			}
			resp.Type = &proto.Response_Init{Init: initResp}
		}
		if plan := req.GetPlan(); plan != nil {
			if !s.initialized {
				return xerrors.New("cannot plan before successful init")
			}
			planResp, err := s.handlePlanRequest(plan, requests)
			if err != nil {
				return err
			}
			if planResp.Error == "" {
				planned = true
			}
			resp.Type = &proto.Response_Plan{Plan: planResp}
		}
		if apply := req.GetApply(); apply != nil {
			if !planned {
				return xerrors.New("cannot apply before successful plan")
			}
			r := &request[*proto.ApplyRequest, *proto.ApplyComplete]{
				req:      apply,
				session:  s,
				serverFn: s.server.Apply,
				cancels:  requests,
			}
			complete, err := r.do()
			if err != nil {
				return err
			}
			resp.Type = &proto.Response_Apply{Apply: complete}
		}
		if graph := req.GetGraph(); graph != nil {
			if !s.initialized {
				return xerrors.New("cannot graph before successful init")
			}

			r := &request[*proto.GraphRequest, *proto.GraphComplete]{
				req:      graph,
				session:  s,
				serverFn: s.server.Graph,
				cancels:  requests,
			}
			complete, err := r.do()
			if err != nil {
				return err
			}
			resp.Type = &proto.Response_Graph{Graph: complete}
		}
		err := s.stream.Send(resp)
		if err != nil {
			return xerrors.Errorf("send response: %w", err)
		}
	}
	return nil
}

func (s *Session) handleInitRequest(init *proto.InitRequest, requests <-chan *proto.Request) (*proto.InitComplete, error) {
	r := &request[*proto.InitRequest, *proto.InitComplete]{
		req:      init,
		session:  s,
		serverFn: s.server.Init,
		cancels:  requests,
	}
	complete, err := r.do()
	if err != nil {
		return nil, err
	}
	if complete.Error != "" {
		return complete, nil
	}

	// If the size of the complete message is too large, we need to stream the module files separately.
	if protobuf.Size(&proto.Response{Type: &proto.Response_Init{Init: complete}}) > drpcsdk.MaxMessageSize {
		// It is likely the modules that is pushing the message size over the limit.
		// Send the modules over a stream of messages instead.
		s.Logger.Info(s.Context(), "plan response too large, sending modules as stream",
			slog.F("size_bytes", len(complete.ModuleFiles)),
		)
		dataUp, chunks := proto.BytesToDataUpload(proto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, complete.ModuleFiles)

		complete.ModuleFiles = nil // sent over the stream
		complete.ModuleFilesHash = dataUp.DataHash

		err := s.stream.Send(&proto.Response{Type: &proto.Response_DataUpload{DataUpload: dataUp}})
		if err != nil {
			complete.Error = fmt.Sprintf("send data upload: %s", err.Error())
		} else {
			for i, chunk := range chunks {
				err := s.stream.Send(&proto.Response{Type: &proto.Response_ChunkPiece{ChunkPiece: chunk}})
				if err != nil {
					complete.Error = fmt.Sprintf("send data piece upload %d/%d: %s", i, dataUp.Chunks, err.Error())
					break
				}
			}
		}
	}
	s.initialized = true

	return complete, nil
}

func (s *Session) handlePlanRequest(plan *proto.PlanRequest, requests <-chan *proto.Request) (*proto.PlanComplete, error) {
	r := &request[*proto.PlanRequest, *proto.PlanComplete]{
		req:      plan,
		session:  s,
		serverFn: s.server.Plan,
		cancels:  requests,
	}
	complete, err := r.do()
	if err != nil {
		return nil, err
	}

	return complete, nil
}

type Session struct {
	Logger slog.Logger
	Files  tfpath.Layout
	Config *proto.Config

	// initialized indicates if an init was run.
	// Required for plan/apply.
	initialized bool

	server   Server
	stream   proto.DRPCProvisioner_SessionStream
	logLevel int32
}

func (s *Session) Context() context.Context {
	return s.stream.Context()
}

func (s *Session) ProvisionLog(level proto.LogLevel, output string) {
	if int32(level) < s.logLevel {
		return
	}

	err := s.stream.Send(&proto.Response{Type: &proto.Response_Log{Log: &proto.Log{
		Level:  level,
		Output: output,
	}}})
	if err != nil {
		s.Logger.Error(s.Context(), "failed to transmit log",
			slog.F("level", level), slog.F("output", output))
	}
}

type pRequest interface {
	*proto.ParseRequest | *proto.InitRequest | *proto.PlanRequest | *proto.ApplyRequest | *proto.GraphRequest
}

type pComplete interface {
	*proto.ParseComplete | *proto.InitComplete | *proto.PlanComplete | *proto.ApplyComplete | *proto.GraphComplete
}

// request processes a single request call to the Server and returns its complete result, while also processing cancel
// requests from the daemon.  Provisioner implementations read from canceledOrComplete to be asynchronously informed
// of cancel.
type request[R pRequest, C pComplete] struct {
	req      R
	session  *Session
	cancels  <-chan *proto.Request
	serverFn func(*Session, R, <-chan struct{}) C
}

func (r *request[R, C]) do() (C, error) {
	canceledOrComplete := make(chan struct{})
	result := make(chan C)
	go func() {
		c := r.serverFn(r.session, r.req, canceledOrComplete)
		result <- c
	}()
	select {
	case req := <-r.cancels:
		close(canceledOrComplete)
		// wait for server to complete the request, even though we have canceled,
		// so that we can't start a new request, and so that if the job was close
		// to completion and the cancel was ignored, we return to complete.
		c := <-result
		// verify we got a cancel instead of another request or closed channel --- which is an error!
		if req.GetCancel() != nil {
			return c, nil
		}
		if req == nil {
			return c, xerrors.New("got nil while old request still processing")
		}
		return c, xerrors.Errorf("got new request %T while old request still processing", req.Type)
	case c := <-result:
		close(canceledOrComplete)
		return c, nil
	}
}
