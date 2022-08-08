package cliui_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/cliui"
)

type tableTest1 struct {
	Name        string      `table:"name"`
	NotIncluded string      // no table tag
	Age         int         `table:"age"`
	Roles       []string    `table:"roles"`
	Sub1        tableTest2  `table:"sub 1,recursive"`
	Sub2        *tableTest2 `table:"sub 2,recursive"`
	Sub3        tableTest3  `table:"sub 3,recursive"`

	// Types with special formatting.
	Time    time.Time  `table:"time"`
	TimePtr *time.Time `table:"time ptr"`
}

type tableTest2 struct {
	Name        string `table:"name"`
	Age         int    `table:"age"`
	NotIncluded string // no table tag
}

type tableTest3 struct {
	NotIncluded string     // no table tag
	Sub         tableTest2 `table:"inner,recursive"`
}

func Test_DisplayTable(t *testing.T) {
	t.Parallel()

	// This test tests skipping fields without table tags, recursion, pointer
	// dereferencing, and nil pointer skipping.
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		someTime := time.Date(2022, 8, 2, 15, 49, 10, 0, time.Local)
		in := []tableTest1{
			{
				Name:  "foo",
				Age:   10,
				Roles: []string{"a", "b", "c"},
				Sub1: tableTest2{
					Name: "foo1",
					Age:  11,
				},
				Sub2: &tableTest2{
					Name: "foo2",
					Age:  12,
				},
				Sub3: tableTest3{
					Sub: tableTest2{
						Name: "foo3",
						Age:  13,
					},
				},
				Time:    someTime,
				TimePtr: &someTime,
			},
			{
				Name:  "bar",
				Age:   20,
				Roles: []string{"a"},
				Sub1: tableTest2{
					Name: "bar1",
					Age:  21,
				},
				Sub2: nil,
				Sub3: tableTest3{
					Sub: tableTest2{
						Name: "bar3",
						Age:  23,
					},
				},
				Time:    someTime,
				TimePtr: nil,
			},
			{
				Name:  "baz",
				Age:   30,
				Roles: nil,
				Sub1: tableTest2{
					Name: "baz1",
					Age:  31,
				},
				Sub2: nil,
				Sub3: tableTest3{
					Sub: tableTest2{
						Name: "baz3",
						Age:  33,
					},
				},
				Time:    someTime,
				TimePtr: nil,
			},
		}

		expected := `
			NAME  AGE  ROLES    SUB 1 NAME  SUB 1 AGE  SUB 2 NAME  SUB 2 AGE  SUB 3 INNER NAME  SUB 3 INNER AGE  TIME             TIME PTR
			foo    10  [a b c]  foo1               11  foo2        12         foo3                           13  Aug  2 15:49:10  Aug  2 15:49:10
			bar    20  [a]      bar1               21  <nil>       <nil>      bar3                           23  Aug  2 15:49:10  <nil>
			baz    30  []       baz1               31  <nil>       <nil>      baz3                           33  Aug  2 15:49:10  <nil>
		`

		// Test with non-pointer values.
		out, err := cliui.DisplayTable(in, "", nil)
		fmt.Println(out)
		require.NoError(t, err)
		compareTables(t, expected, out)

		// Test with pointer values.
		inPtr := make([]*tableTest1, len(in))
		for i, v := range in {
			v := v
			inPtr[i] = &v
		}
		out, err = cliui.DisplayTable(inPtr, "", nil)
		require.NoError(t, err)
		compareTables(t, expected, out)
	})

	// This test ensures that safeties against invalid use of `table` tags
	// causes panics (even without data).
	t.Run("Panics", func(t *testing.T) {
		t.Parallel()

		t.Run("NotSlice", func(t *testing.T) {
			t.Parallel()

			var in string
			require.Panics(t, func() {
				_, _ = cliui.DisplayTable(in, "", nil)
			})
		})

		t.Run("Interfaces", func(t *testing.T) {
			t.Parallel()

			t.Run("WithoutData", func(t *testing.T) {
				t.Parallel()

				var in []interface{}
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})

			t.Run("WithData", func(t *testing.T) {
				t.Parallel()

				in := []interface{}{tableTest1{}}
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})
		})

		t.Run("NotStruct", func(t *testing.T) {
			t.Parallel()

			t.Run("WithoutData", func(t *testing.T) {
				t.Parallel()

				var in []string
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})

			t.Run("WithData", func(t *testing.T) {
				t.Parallel()

				in := []string{"foo", "bar", "baz"}
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})
		})

		t.Run("NoTableTags", func(t *testing.T) {
			t.Parallel()

			type noTableTagsTest struct {
				Field string `json:"field"`
			}

			t.Run("WithoutData", func(t *testing.T) {
				t.Parallel()

				var in []noTableTagsTest
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})

			t.Run("WithData", func(t *testing.T) {
				t.Parallel()

				in := []noTableTagsTest{{Field: "hi"}}
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})
		})

		t.Run("InvalidTag/NoName", func(t *testing.T) {
			t.Parallel()

			type noNameTest struct {
				Field string `table:""`
			}

			t.Run("WithoutData", func(t *testing.T) {
				t.Parallel()

				var in []noNameTest
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})

			t.Run("WithData", func(t *testing.T) {
				t.Parallel()

				in := []noNameTest{{Field: "test"}}
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})
		})

		t.Run("InvalidTag/BadSyntax", func(t *testing.T) {
			t.Parallel()

			type invalidSyntaxTest struct {
				Field string `table:"hi,hi"`
			}

			t.Run("WithoutData", func(t *testing.T) {
				t.Parallel()

				var in []invalidSyntaxTest
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})

			t.Run("WithData", func(t *testing.T) {
				t.Parallel()

				in := []invalidSyntaxTest{{Field: "test"}}
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})
		})

		t.Run("RecurseNonStruct/Raw", func(t *testing.T) {
			t.Parallel()

			type recurseNonStruct struct {
				Field string `table:"field,recursive"`
			}

			t.Run("WithoutData", func(t *testing.T) {
				t.Parallel()

				var in []recurseNonStruct
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})

			t.Run("WithData", func(t *testing.T) {
				t.Parallel()

				in := []recurseNonStruct{{Field: "test"}}
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})
		})

		t.Run("RecurseNonStruct/Pointer", func(t *testing.T) {
			t.Parallel()

			type recurseNonStruct struct {
				Field *string `table:"field,recursive"`
			}

			t.Run("WithoutData", func(t *testing.T) {
				t.Parallel()

				var in []recurseNonStruct
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})

			t.Run("WithData", func(t *testing.T) {
				t.Parallel()

				val := "test"
				in := []recurseNonStruct{{Field: &val}}
				require.Panics(t, func() {
					_, _ = cliui.DisplayTable(in, "", nil)
				})
			})
		})
	})
}

// compareTables normalizes the incoming table lines
func compareTables(t *testing.T, expected, out string) {
	t.Helper()

	expectedLines := strings.Split(strings.TrimSpace(expected), "\n")
	gotLines := strings.Split(strings.TrimSpace(out), "\n")
	assert.Equal(t, len(expectedLines), len(gotLines), "expected line count does not match generated line count")

	// Map the expected and got lines to normalize them.
	expectedNormalized := make([]string, len(expectedLines))
	gotNormalized := make([]string, len(gotLines))
	normalizeLine := func(s string) string {
		return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	}
	for i, s := range expectedLines {
		expectedNormalized[i] = normalizeLine(s)
	}
	for i, s := range gotLines {
		gotNormalized[i] = normalizeLine(s)
	}

	require.Equal(t, expectedNormalized, gotNormalized, "expected lines to match generated lines")
}
