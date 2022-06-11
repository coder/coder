package provisionersdk

import (
	"bufio"
	"io"

	"github.com/coder/coder/provisionersdk/proto"
)

type Logger interface {
	Log(l *proto.Log) error
}

type ProvisionStream interface {
	Send(*proto.Provision_Response) error
}

type ParseStream interface {
	Send(response *proto.Parse_Response) error
}

func NewProvisionLogger(s ProvisionStream) Logger {
	return provisionLogger{s}
}

func NewParseLogger(s ParseStream) Logger {
	return parseLogger{s}
}

type provisionLogger struct {
	stream ProvisionStream
}

func (p provisionLogger) Log(l *proto.Log) error {
	return p.stream.Send(&proto.Provision_Response{
		Type: &proto.Provision_Response_Log{
			Log: l,
		},
	})
}

type parseLogger struct {
	stream ParseStream
}

func (p parseLogger) Log(l *proto.Log) error {
	return p.stream.Send(&proto.Parse_Response{
		Type: &proto.Parse_Response_Log{
			Log: l,
		},
	})
}

func LogWriter(logger Logger, level proto.LogLevel) (io.WriteCloser, <-chan any) {
	r, w := io.Pipe()
	done := make(chan any)
	go readAndLog(logger, r, done, level)
	return w, done
}

func readAndLog(logger Logger, r io.Reader, done chan<- any, level proto.LogLevel) {
	defer close(done)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		err := logger.Log(&proto.Log{Level: level, Output: scanner.Text()})
		if err != nil {
			// Not much we can do.  We can't log because logging is itself breaking!
			return
		}
	}
}
