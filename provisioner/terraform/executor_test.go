// nolint:testpackage
package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk/proto"
)

type mockLogger struct {
	logs   []*proto.Log
	retVal error
}

func (m *mockLogger) Log(l *proto.Log) error {
	m.logs = append(m.logs, l)
	return m.retVal
}

func TestLogWriter_Mainline(t *testing.T) {
	t.Parallel()

	logr := &mockLogger{retVal: nil}
	writer, doneLogging := logWriter(logr, proto.LogLevel_INFO)

	_, err := writer.Write([]byte(`Sitting in an English garden
Waiting for the sun
If the sun don't come you get a tan
From standing in the English rain`))
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)
	<-doneLogging

	expected := []*proto.Log{
		{Level: proto.LogLevel_INFO, Output: "Sitting in an English garden"},
		{Level: proto.LogLevel_INFO, Output: "Waiting for the sun"},
		{Level: proto.LogLevel_INFO, Output: "If the sun don't come you get a tan"},
		{Level: proto.LogLevel_INFO, Output: "From standing in the English rain"},
	}
	require.Equal(t, logr.logs, expected)
}

func TestLogWriter_SendError(t *testing.T) {
	t.Parallel()

	logr := &mockLogger{retVal: xerrors.New("Goo goo g'joob")}
	writer, doneLogging := logWriter(logr, proto.LogLevel_INFO)

	_, err := writer.Write([]byte(`Sitting in an English garden
Waiting for the sun
If the sun don't come you get a tan
From standing in the English rain`))
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)
	<-doneLogging
	expected := []*proto.Log{{Level: proto.LogLevel_INFO, Output: "Sitting in an English garden"}}
	require.Equal(t, logr.logs, expected)
}
