package provisionersdk_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/mocks"
	"github.com/coder/coder/provisionersdk/proto"
)

type logMatcher struct {
	level  proto.LogLevel
	output string
}

func (m logMatcher) Matches(x interface{}) bool {
	switch r := x.(type) {
	case *proto.Provision_Response:
		return m.logMatches(r.GetLog())
	case *proto.Parse_Response:
		return m.logMatches(r.GetLog())
	default:
		return false
	}
}

func (m logMatcher) logMatches(l *proto.Log) bool {
	if l.Level != m.level {
		return false
	}
	if l.Output != m.output {
		return false
	}
	return true
}

func withLog(log *proto.Log) func(x interface{}) bool {
	m := logMatcher{level: log.GetLevel(), output: log.GetOutput()}
	return m.Matches
}

func TestProvisionLogger_Log(t *testing.T) {
	t.Parallel()

	mStream := new(mocks.ProvisionStream)
	defer mStream.AssertExpectations(t)

	l := &proto.Log{Level: proto.LogLevel_ERROR, Output: "an error"}
	mStream.On("Send", mock.MatchedBy(withLog(l))).Return(nil)

	uut := provisionersdk.NewProvisionLogger(mStream)
	err := uut.Log(l)
	require.NoError(t, err)
}

func TestParseLogger_Log(t *testing.T) {
	t.Parallel()

	mStream := new(mocks.ParseStream)
	defer mStream.AssertExpectations(t)

	l := &proto.Log{Level: proto.LogLevel_ERROR, Output: "an error"}
	mStream.On("Send", mock.MatchedBy(withLog(l))).Return(nil)

	uut := provisionersdk.NewParseLogger(mStream)
	err := uut.Log(l)
	require.NoError(t, err)
}

func TestLogWriter_Mainline(t *testing.T) {
	t.Parallel()

	mStream := new(mocks.ParseStream)
	defer mStream.AssertExpectations(t)

	logger := provisionersdk.NewParseLogger(mStream)
	writer, doneLogging := provisionersdk.LogWriter(logger, proto.LogLevel_INFO)

	expected := []*proto.Log{
		{Level: proto.LogLevel_INFO, Output: "Sitting in an English garden"},
		{Level: proto.LogLevel_INFO, Output: "Waiting for the sun"},
		{Level: proto.LogLevel_INFO, Output: "If the sun don't come you get a tan"},
		{Level: proto.LogLevel_INFO, Output: "From standing in the English rain"},
	}
	for _, log := range expected {
		mStream.On("Send", mock.MatchedBy(withLog(log))).Return(nil)
	}

	_, err := writer.Write([]byte(`Sitting in an English garden
Waiting for the sun
If the sun don't come you get a tan
From standing in the English rain`))
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)
	<-doneLogging
}

func TestLogWriter_SendError(t *testing.T) {
	t.Parallel()

	mStream := new(mocks.ParseStream)
	defer mStream.AssertExpectations(t)

	logger := provisionersdk.NewParseLogger(mStream)
	writer, doneLogging := provisionersdk.LogWriter(logger, proto.LogLevel_INFO)

	expected := &proto.Log{Level: proto.LogLevel_INFO, Output: "Sitting in an English garden"}
	mStream.On("Send", mock.MatchedBy(withLog(expected))).Return(xerrors.New("Goo goo g'joob"))

	_, err := writer.Write([]byte(`Sitting in an English garden
Waiting for the sun
If the sun don't come you get a tan
From standing in the English rain`))
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)
	<-doneLogging
}
