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

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisionersdk/tfpath"

	protobuf "google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

const (
	// ReadmeFile is the location we look for to extract documentation from template versions.
	ReadmeFile = "README.md"

	sessionDirPrefix      = "Session"
	staleSessionRetention = 7 * 24 * time.Hour
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

	err := CleanStaleSessions(s.Context(), p.opts.WorkDirectory, afero.NewOsFs(), time.Now(), s.Logger)
	if err != nil {
		return xerrors.Errorf("unable to clean stale sessions %q: %w", s.Files, err)
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

	err = s.Files.ExtractArchive(s.Context(), s.Logger, afero.NewOsFs(), s.Config)
	if err != nil {
		return xerrors.Errorf("extract archive: %w", err)
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
		if plan := req.GetPlan(); plan != nil {
			r := &request[*proto.PlanRequest, *proto.PlanComplete]{
				req:      plan,
				session:  s,
				serverFn: s.server.Plan,
				cancels:  requests,
			}
			complete, err := r.do()
			if err != nil {
				return err
			}
			resp.Type = &proto.Response_Plan{Plan: complete}

			if protobuf.Size(resp) > drpcsdk.MaxMessageSize {
				// It is likely the modules that is pushing the message size over the limit.
				// Send the modules over a stream of messages instead.
				s.Logger.Info(s.Context(), "plan response too large, sending modules as stream",
					slog.F("size_bytes", len(complete.ModuleFiles)),
				)
				dataUp, chunks := proto.BytesToDataUpload(proto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, complete.ModuleFiles)

				complete.ModuleFiles = nil // sent over the stream
				complete.ModuleFilesHash = dataUp.DataHash
				resp.Type = &proto.Response_Plan{Plan: complete}

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

			if complete.Error == "" {
				planned = true
			}
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
		err := s.stream.Send(resp)
		if err != nil {
			return xerrors.Errorf("send response: %w", err)
		}
	}
	return nil
}

type Session struct {
	Logger slog.Logger
	Files  tfpath.Layout
	Config *proto.Config

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
	*proto.ParseRequest | *proto.PlanRequest | *proto.ApplyRequest
}

type pComplete interface {
	*proto.ParseComplete | *proto.PlanComplete | *proto.ApplyComplete
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
