package dbtestutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCopyStrategyClauseForVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		versionNum int
		want       string
	}{
		{name: "PostgreSQL13HasNoStrategyOption", versionNum: 130022, want: ""},
		{name: "PostgreSQL14HasNoStrategyOption", versionNum: 140019, want: ""},
		{name: "PostgreSQL15UsesFileCopy", versionNum: 150000, want: " STRATEGY FILE_COPY"},
		{name: "PostgreSQL17UsesFileCopy", versionNum: 170006, want: " STRATEGY FILE_COPY"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, copyStrategyClauseForVersion(tc.versionNum))
		})
	}
}
