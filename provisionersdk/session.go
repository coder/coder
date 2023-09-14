package provisionersdk

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

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
		return xerrors.Errorf("unable to clean stale sessions %q: %w", s.WorkDirectory, err)
	}

	s.WorkDirectory = filepath.Join(p.opts.WorkDirectory, SessionDir(sessID))
	err = os.MkdirAll(s.WorkDirectory, 0o700)
	if err != nil {
		return xerrors.Errorf("create work directory %q: %w", s.WorkDirectory, err)
	}
	defer func() {
		var err error
		// Cleanup the work directory after execution.
		for attempt := 0; attempt < 5; attempt++ {
			err = os.RemoveAll(s.WorkDirectory)
			if err != nil {
				// On Windows, open files cannot be removed.
				// When the provisioner daemon is shutting down,
				// it may take a few milliseconds for processes to exit.
				// See: https://github.com/golang/go/issues/50510
				s.Logger.Debug(s.Context(), "failed to clean work directory; trying again", slog.Error(err))
				time.Sleep(250 * time.Millisecond)
				continue
			}
			s.Logger.Debug(s.Context(), "cleaned up work directory")
			return
		}
		s.Logger.Error(s.Context(), "failed to clean up work directory after multiple attempts",
			slog.F("path", s.WorkDirectory), slog.Error(err))
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

	err = s.extractArchive()
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
				s.Logger.Info(s.Context(), "recv done on Session", slog.Error(err))
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
			readme, err := os.ReadFile(filepath.Join(s.WorkDirectory, ReadmeFile))
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
	Logger        slog.Logger
	WorkDirectory string
	Config        *proto.Config

	server   Server
	stream   proto.DRPCProvisioner_SessionStream
	logLevel int32
}

func (s *Session) Context() context.Context {
	return s.stream.Context()
}

func (s *Session) extractArchive() error {
	ctx := s.Context()

	s.Logger.Info(ctx, "unpacking template source archive",
		slog.F("size_bytes", len(s.Config.TemplateSourceArchive)),
	)

	reader := tar.NewReader(bytes.NewBuffer(s.Config.TemplateSourceArchive))
	// for safety, nil out the reference on Config, since the reader now owns it.
	s.Config.TemplateSourceArchive = nil
	for {
		header, err := reader.Next()
		if err != nil {
			if xerrors.Is(err, io.EOF) {
				break
			}
			return xerrors.Errorf("read template source archive: %w", err)
		}
		// Security: don't untar absolute or relative paths, as this can allow a malicious tar to overwrite
		// files outside the workdir.
		if !filepath.IsLocal(header.Name) {
			return xerrors.Errorf("refusing to extract to non-local path")
		}
		// nolint: gosec
		headerPath := filepath.Join(s.WorkDirectory, header.Name)
		if !strings.HasPrefix(headerPath, filepath.Clean(s.WorkDirectory)) {
			return xerrors.New("tar attempts to target relative upper directory")
		}
		mode := header.FileInfo().Mode()
		if mode == 0 {
			mode = 0o600
		}

		// Always check for context cancellation before reading the next header.
		// This is mainly important for unit tests, since a canceled context means
		// the underlying directory is going to be deleted. There still exists
		// the small race condition that the context is cancelled after this, and
		// before the disk write.
		if ctx.Err() != nil {
			return xerrors.Errorf("context canceled: %w", ctx.Err())
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(headerPath, mode)
			if err != nil {
				return xerrors.Errorf("mkdir %q: %w", headerPath, err)
			}
			s.Logger.Debug(context.Background(), "extracted directory",
				slog.F("path", headerPath),
				slog.F("mode", fmt.Sprintf("%O", mode)))
		case tar.TypeReg:
			file, err := os.OpenFile(headerPath, os.O_CREATE|os.O_RDWR, mode)
			if err != nil {
				return xerrors.Errorf("create file %q (mode %s): %w", headerPath, mode, err)
			}
			// Max file size of 10MiB.
			size, err := io.CopyN(file, reader, 10<<20)
			if xerrors.Is(err, io.EOF) {
				err = nil
			}
			if err != nil {
				_ = file.Close()
				return xerrors.Errorf("copy file %q: %w", headerPath, err)
			}
			err = file.Close()
			if err != nil {
				return xerrors.Errorf("close file %q: %s", headerPath, err)
			}
			s.Logger.Debug(context.Background(), "extracted file",
				slog.F("size_bytes", size),
				slog.F("path", headerPath),
				slog.F("mode", mode),
			)
		}
	}
	return nil
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

// SessionDir returns the directory name with mandatory prefix.
func SessionDir(sessID string) string {
	return sessionDirPrefix + sessID
}
