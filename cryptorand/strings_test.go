package cryptorand_test

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cryptorand"
)

func TestString(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		rs, err := cryptorand.String(10)
		require.NoError(t, err, "unexpected error from String")
		t.Logf("value: %v <- random?", rs)
	}
}

func TestStringCharset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name       string
		Charset    string
		HelperFunc func(int) (string, error)
		Length     int
	}{
		{
			Name:    "MultiByte-20",
			Charset: "💓😘💓🌷",
			Length:  20,
		},
		{
			Name:    "MultiByte-7",
			Charset: "😇🥰😍🤩😘😗☺️😚😙🥲😋😛😜🤪😝🤑",
			Length:  7,
		},
		{
			Name:    "MixedBytes",
			Charset: "🍋🍌🍍🥭🍎🍏🍐🍑🍒🍓🫐🥝🍅🫒🥥🥑🍆🥔abcdefg1234",
			Length:  10,
		},
		{
			Name:       "Empty",
			Charset:    cryptorand.Default,
			Length:     0,
			HelperFunc: cryptorand.String,
		},
		{
			Name:    "Numeric",
			Charset: cryptorand.Numeric,
			Length:  1,
		},
		{
			Name:    "Upper",
			Charset: cryptorand.Upper,
			Length:  3,
		},
		{
			Name:    "Lower",
			Charset: cryptorand.Lower,
			Length:  10,
		},
		{
			Name:    "Alpha",
			Charset: cryptorand.Alpha,
			Length:  20,
		},
		{
			Name:    "Default",
			Charset: cryptorand.Default,
			Length:  10,
		},
		{
			Name:       "Hex",
			Charset:    cryptorand.Hex,
			Length:     15,
			HelperFunc: cryptorand.HexString,
		},
		{
			Name:    "Human",
			Charset: cryptorand.Human,
			Length:  20,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()

			for i := 0; i < 5; i++ {
				rs, err := cryptorand.StringCharset(test.Charset, test.Length)
				require.NoError(t, err, "unexpected error from StringCharset")
				require.Equal(t, test.Length, utf8.RuneCountInString(rs), "expected RuneCountInString to match requested")
				if i == 0 {
					t.Logf("value: %v <- random?", rs)
				}
			}
		})

		if test.HelperFunc != nil {
			t.Run(test.Name+"HelperFunc", func(t *testing.T) {
				t.Parallel()

				for i := 0; i < 5; i++ {
					rs, err := test.HelperFunc(test.Length)
					require.NoError(t, err, "unexpected error from HelperFunc")
					require.Equal(t, test.Length, utf8.RuneCountInString(rs), "expected RuneCountInString to match requested")
					if i == 0 {
						t.Logf("value: %v <- random?", rs)
					}
				}
			})
		}
	}
}

func TestSha1String(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		rs, err := cryptorand.Sha1String()
		require.NoError(t, err, "unexpected error from String")
		require.Equal(t, 40, utf8.RuneCountInString(rs), "expected RuneCountInString to match requested")
		t.Logf("value: %v <- random?", rs)
	}
}

func BenchmarkString20(b *testing.B) {
	b.SetBytes(20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = cryptorand.String(20)
	}
}

func BenchmarkStringUnsafe20(b *testing.B) {
	mkstring := func(charSetStr string, size int) (string, error) {
		charSet := []rune(charSetStr)

		// This buffer facilitates pre-emptively creation of random uint32s
		// to reduce syscall overhead.
		ibuf := make([]byte, 4*size)

		_, err := rand.Read(ibuf)
		if err != nil {
			return "", err
		}

		var buf strings.Builder
		buf.Grow(size)

		for i := 0; i < size; i++ {
			n := binary.BigEndian.Uint32(ibuf[i*4 : (i+1)*4])
			_, _ = buf.WriteRune(charSet[n%uint32(len(charSet))]) // #nosec G115 - Safe conversion as len(charSet) will be reasonably small for character sets
		}

		return buf.String(), nil
	}

	b.SetBytes(20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = mkstring(cryptorand.Default, 20)
	}
}

func BenchmarkStringBigint20(b *testing.B) {
	mkstring := func(charSetStr string, size int) (string, error) {
		charSet := []rune(charSetStr)

		var buf strings.Builder
		buf.Grow(size)

		bi := big.NewInt(int64(size))
		for i := 0; i < size; i++ {
			num, err := rand.Int(rand.Reader, bi)
			if err != nil {
				return "", err
			}

			_, _ = buf.WriteRune(charSet[num.Uint64()%uint64(len(charSet))])
		}

		return buf.String(), nil
	}

	b.SetBytes(20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = mkstring(cryptorand.Default, 20)
	}
}

func BenchmarkStringRuneCast(b *testing.B) {
	s := strings.Repeat("0", 20)
	b.SetBytes(int64(len(s)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = []rune(s)
	}
}
