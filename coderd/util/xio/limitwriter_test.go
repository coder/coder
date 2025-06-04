package xio_test

import (
	"bytes"
	cryptorand "crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/xio"
)

func TestLimitWriter(t *testing.T) {
	t.Parallel()

	type writeCase struct {
		N    int
		ExpN int
		Err  bool
	}

	// testCases will do multiple writes to the same limit writer and check the output.
	testCases := []struct {
		Name   string
		L      int64
		Writes []writeCase
		N      int
		ExpN   int
	}{
		{
			Name: "Empty",
			L:    1000,
			Writes: []writeCase{
				// A few empty writes
				{N: 0, ExpN: 0}, {N: 0, ExpN: 0}, {N: 0, ExpN: 0},
			},
		},
		{
			Name: "NotFull",
			L:    1000,
			Writes: []writeCase{
				{N: 250, ExpN: 250},
				{N: 250, ExpN: 250},
				{N: 250, ExpN: 250},
			},
		},
		{
			Name: "Short",
			L:    1000,
			Writes: []writeCase{
				{N: 250, ExpN: 250},
				{N: 250, ExpN: 250},
				{N: 250, ExpN: 250},
				{N: 250, ExpN: 250},
				{N: 250, ExpN: 0, Err: true},
			},
		},
		{
			Name: "Exact",
			L:    1000,
			Writes: []writeCase{
				{
					N:    1000,
					ExpN: 1000,
				},
				{
					N:   1000,
					Err: true,
				},
			},
		},
		{
			Name: "Over",
			L:    1000,
			Writes: []writeCase{
				{
					N:    5000,
					ExpN: 0,
					Err:  true,
				},
				{
					N:   5000,
					Err: true,
				},
				{
					N:   5000,
					Err: true,
				},
			},
		},
		{
			Name: "Strange",
			L:    -1,
			Writes: []writeCase{
				{
					N:    5,
					ExpN: 0,
					Err:  true,
				},
				{
					N:    0,
					ExpN: 0,
					Err:  true,
				},
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			buf := bytes.NewBuffer([]byte{})
			allBuff := bytes.NewBuffer([]byte{})
			w := xio.NewLimitWriter(buf, c.L)

			for _, wc := range c.Writes {
				data := make([]byte, wc.N)

				n, err := cryptorand.Read(data)
				require.NoError(t, err, "crand read")
				require.Equal(t, wc.N, n, "correct bytes read")
				maxSeen := data[:wc.ExpN]
				n, err = w.Write(data)
				if wc.Err {
					require.Error(t, err, "exp error")
				} else {
					require.NoError(t, err, "write")
				}

				// Need to use this to compare across multiple writes.
				// Each write appends to the expected output.
				allBuff.Write(maxSeen)

				require.Equal(t, wc.ExpN, n, "correct bytes written")
				require.Equal(t, allBuff.Bytes(), buf.Bytes(), "expected data")
			}
		})
	}
}
