package terraform

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

type mockLogger struct {
	logs []*proto.Log
}

var _ logSink = &mockLogger{}

func (m *mockLogger) Log(l *proto.Log) {
	m.logs = append(m.logs, l)
}

func TestLogWriter_Mainline(t *testing.T) {
	t.Parallel()

	logr := &mockLogger{}
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
	require.Equal(t, expected, logr.logs)
}

func TestOnlyDataResources(t *testing.T) {
	t.Parallel()

	sm := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			{
				Name:    "cat",
				Type:    "coder_parameter",
				Mode:    "data",
				Address: "cat-address",
			},
			{
				Name:    "dog",
				Type:    "foobar",
				Mode:    "magic",
				Address: "dog-address",
			},
			{
				Name:    "cow",
				Type:    "foobaz",
				Mode:    "data",
				Address: "cow-address",
			},
		},
		ChildModules: []*tfjson.StateModule{
			{
				Resources: []*tfjson.StateResource{
					{
						Name:    "child-cat",
						Type:    "coder_parameter",
						Mode:    "data",
						Address: "child-cat-address",
					},
					{
						Name:    "child-dog",
						Type:    "foobar",
						Mode:    "data",
						Address: "child-dog-address",
					},
					{
						Name:    "child-cow",
						Mode:    "data",
						Type:    "magic",
						Address: "child-cow-address",
					},
				},
				Address: "child-module-1",
			},
		},
		Address: "fake-module",
	}

	filtered := onlyDataResources(*sm)
	require.Len(t, filtered.Resources, 2)
	require.Len(t, filtered.ChildModules, 1)
}
