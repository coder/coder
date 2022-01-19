package longid

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestID(t *testing.T) {
	last, next := TimeReset()
	t.Logf("Long Reset: Last: %v, Next: %v (🔼 %v)\n", last, next, next.Sub(last))
	t.Run("New()", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			l := New()
			fmt.Printf("Long: %v\n", l)
			assert.WithinDuration(t, time.Now(), l.CreatedAt(), time.Second)
		}
	})

	t.Run("Parse()", func(t *testing.T) {
		t.Run("Good", func(t *testing.T) {
			want := New()
			got, err := Parse(want.String())
			require.Nil(t, err)
			require.Equal(t, want, got)
		})

		t.Run("Bad Size", func(t *testing.T) {
			_, err := Parse(New().String() + "ab")
			require.NotNil(t, err)
		})

		t.Run("Bad Hex", func(t *testing.T) {
			str := New().String()
			str = "O" + str[1:]
			_, err := Parse(str)
			require.NotNil(t, err)
		})
	})

	t.Run("FromSlice", func(t *testing.T) {
		l := New()
		assert.Equal(t, l, FromSlice(l[:]))
	})

	t.Run("Scan", func(t *testing.T) {
		var l ID
		b := make([]byte, 16)
		_, err := rand.Read(b)
		require.NoError(t, err)

		require.NoError(t, l.Scan(b))
		assert.Equal(t, b, l.Bytes())
	})
}

func TestLongRaces(_ *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		go func() {
			for i := 0; i < 1000; i++ {
				New()
			}
		}()
	}
	wg.Wait()
}

func BenchmarkLong(b *testing.B) {
	b.Run("New()", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			New()
		}
	})
}
