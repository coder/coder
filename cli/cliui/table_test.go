package cliui_test

import (
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/cliui"
)

type stringWrapper struct {
	str string
}

var _ fmt.Stringer = stringWrapper{}

func (s stringWrapper) String() string {
	return s.str
}

type tableTest1 struct {
	Name        string      `table:"name,default_sort"`
	NotIncluded string      // no table tag
	Age         int         `table:"age"`
	Roles       []string    `table:"roles"`
	Sub1        tableTest2  `table:"sub_1,recursive"`
	Sub2        *tableTest2 `table:"sub_2,recursive"`
	Sub3        tableTest3  `table:"sub 3,recursive"`
	Sub4        tableTest2  `table:"sub 4"` // not recursive

	// Types with special formatting.
	Time    time.Time  `table:"time"`
	TimePtr *time.Time `table:"time_ptr"`
}

type tableTest2 struct {
	Name        stringWrapper `table:"name,default_sort"`
	Age         int           `table:"age"`
	NotIncluded string        `table:"-"`
}

type tableTest3 struct {
	NotIncluded string     // no table tag
	Sub         tableTest2 `table:"inner,recursive"`
}

type tableTest4 struct {
	Inline    tableTest2 `table:"ignored,recursive_inline"`
	SortField string     `table:"sort_field"`
}

func Test_DisplayTable(t *testing.T) {
	t.Parallel()

	someTime := time.Date(2022, 8, 2, 15, 49, 10, 0, time.UTC)

	// Not sorted by name or age to test sorting.
	in := []tableTest1{
		{
			Name:  "bar",
			Age:   20,
			Roles: []string{"a"},
			Sub1: tableTest2{
				Name: stringWrapper{str: "bar1"},
				Age:  21,
			},
			Sub2: nil,
			Sub3: tableTest3{
				Sub: tableTest2{
					Name: stringWrapper{str: "bar3"},
					Age:  23,
				},
			},
			Sub4: tableTest2{
				Name: stringWrapper{str: "bar4"},
				Age:  24,
			},
			Time:    someTime,
			TimePtr: nil,
		},
		{
			Name:  "foo",
			Age:   10,
			Roles: []string{"a", "b", "c"},
			Sub1: tableTest2{
				Name: stringWrapper{str: "foo1"},
				Age:  11,
			},
			Sub2: &tableTest2{
				Name: stringWrapper{str: "foo2"},
				Age:  12,
			},
			Sub3: tableTest3{
				Sub: tableTest2{
					Name: stringWrapper{str: "foo3"},
					Age:  13,
				},
			},
			Sub4: tableTest2{
				Name: stringWrapper{str: "foo4"},
				Age:  14,
			},
			Time:    someTime,
			TimePtr: &someTime,
		},
		{
			Name:  "baz",
			Age:   30,
			Roles: nil,
			Sub1: tableTest2{
				Name: stringWrapper{str: "baz1"},
				Age:  31,
			},
			Sub2: nil,
			Sub3: tableTest3{
				Sub: tableTest2{
					Name: stringWrapper{str: "baz3"},
					Age:  33,
				},
			},
			Sub4: tableTest2{
				Name: stringWrapper{str: "baz4"},
				Age:  34,
			},
			Time:    someTime,
			TimePtr: nil,
		},
	}

	// This test tests skipping fields without table tags, recursion, pointer
	// dereferencing, and nil pointer skipping.
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		expected := `
NAME  AGE  ROLES    SUB 1 NAME  SUB 1 AGE  SUB 2 NAME  SUB 2 AGE  SUB 3 INNER NAME  SUB 3 INNER AGE  SUB 4       TIME                  TIME PTR
bar    20  [a]      bar1               21  <nil>       <nil>      bar3                           23  {bar4 24 }  2022-08-02T15:49:10Z  <nil>
baz    30  []       baz1               31  <nil>       <nil>      baz3                           33  {baz4 34 }  2022-08-02T15:49:10Z  <nil>
foo    10  [a b c]  foo1               11  foo2        12         foo3                           13  {foo4 14 }  2022-08-02T15:49:10Z  2022-08-02T15:49:10Z
		`

		// Test with non-pointer values.
		out, err := cliui.DisplayTable(in, "", nil)
		log.Println("rendered table:\n" + out)
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

	t.Run("CustomSort", func(t *testing.T) {
		t.Parallel()

		expected := `
NAME  AGE  ROLES    SUB 1 NAME  SUB 1 AGE  SUB 2 NAME  SUB 2 AGE  SUB 3 INNER NAME  SUB 3 INNER AGE  SUB 4       TIME                  TIME PTR
foo    10  [a b c]  foo1               11  foo2        12         foo3                           13  {foo4 14 }  2022-08-02T15:49:10Z  2022-08-02T15:49:10Z
bar    20  [a]      bar1               21  <nil>       <nil>      bar3                           23  {bar4 24 }  2022-08-02T15:49:10Z  <nil>
baz    30  []       baz1               31  <nil>       <nil>      baz3                           33  {baz4 34 }  2022-08-02T15:49:10Z  <nil>
		`

		out, err := cliui.DisplayTable(in, "age", nil)
		log.Println("rendered table:\n" + out)
		require.NoError(t, err)
		compareTables(t, expected, out)
	})

	t.Run("Filter", func(t *testing.T) {
		t.Parallel()

		expected := `
NAME  SUB 1 NAME  SUB 3 INNER NAME  TIME
bar   bar1        bar3              2022-08-02T15:49:10Z
baz   baz1        baz3              2022-08-02T15:49:10Z
foo   foo1        foo3              2022-08-02T15:49:10Z
		`

		out, err := cliui.DisplayTable(in, "", []string{"name", "sub_1_name", "sub_3 inner name", "time"})
		log.Println("rendered table:\n" + out)
		require.NoError(t, err)
		compareTables(t, expected, out)
	})

	t.Run("Inline", func(t *testing.T) {
		t.Parallel()

		expected := `
NAME    AGE
Alice   25
		`

		inlineIn := []tableTest4{
			{
				Inline: tableTest2{
					Name: stringWrapper{
						str: "Alice",
					},
					Age:         25,
					NotIncluded: "IgnoreMe",
				},
			},
		}
		out, err := cliui.DisplayTable(inlineIn, "", []string{"name", "age"})
		log.Println("rendered table:\n" + out)
		require.NoError(t, err)
		compareTables(t, expected, out)
	})

	// This test ensures we can display dynamically typed slices
	t.Run("Interfaces", func(t *testing.T) {
		t.Parallel()

		in := []any{tableTest1{}}
		out, err := cliui.DisplayTable(in, "", nil)
		t.Log("rendered table:\n" + out)
		require.NoError(t, err)
		other := []tableTest1{{}}
		expected, err := cliui.DisplayTable(other, "", nil)
		require.NoError(t, err)
		compareTables(t, expected, out)
	})

	t.Run("WithSeparator", func(t *testing.T) {
		t.Parallel()
		expected := `
NAME  AGE  ROLES    SUB 1 NAME  SUB 1 AGE  SUB 2 NAME  SUB 2 AGE  SUB 3 INNER NAME  SUB 3 INNER AGE  SUB 4       TIME                  TIME PTR              
bar    20  [a]      bar1               21  <nil>       <nil>      bar3                           23  {bar4 24 }  2022-08-02T15:49:10Z  <nil>                 
-------------------------------------------------------------------------------------------------------------------------------------------------------------
baz    30  []       baz1               31  <nil>       <nil>      baz3                           33  {baz4 34 }  2022-08-02T15:49:10Z  <nil>                 
-------------------------------------------------------------------------------------------------------------------------------------------------------------
foo    10  [a b c]  foo1               11  foo2        12         foo3                           13  {foo4 14 }  2022-08-02T15:49:10Z  2022-08-02T15:49:10Z 
		`

		var inlineIn []any
		for _, v := range in {
			inlineIn = append(inlineIn, v)
			inlineIn = append(inlineIn, cliui.TableSeparator{})
		}
		out, err := cliui.DisplayTable(inlineIn, "", nil)
		t.Log("rendered table:\n" + out)
		require.NoError(t, err)
		compareTables(t, expected, out)
	})

	// This test ensures that safeties against invalid use of `table` tags
	// causes errors (even without data).
	t.Run("Errors", func(t *testing.T) {
		t.Parallel()

		t.Run("NotSlice", func(t *testing.T) {
			t.Parallel()

			var in string
			_, err := cliui.DisplayTable(in, "", nil)
			require.Error(t, err)
		})

		t.Run("BadSortColumn", func(t *testing.T) {
			t.Parallel()

			_, err := cliui.DisplayTable(in, "bad_column_does_not_exist", nil)
			require.Error(t, err)
		})

		t.Run("BadFilterColumns", func(t *testing.T) {
			t.Parallel()

			_, err := cliui.DisplayTable(in, "", []string{"name", "bad_column_does_not_exist"})
			require.Error(t, err)
		})

		t.Run("Interfaces", func(t *testing.T) {
			t.Parallel()

			t.Run("WithoutData", func(t *testing.T) {
				t.Parallel()

				var in []any
				_, err := cliui.DisplayTable(in, "", nil)
				require.Error(t, err)
			})
		})

		t.Run("NotStruct", func(t *testing.T) {
			t.Parallel()

			t.Run("WithoutData", func(t *testing.T) {
				t.Parallel()

				var in []string
				_, err := cliui.DisplayTable(in, "", nil)
				require.Error(t, err)
			})

			t.Run("WithData", func(t *testing.T) {
				t.Parallel()

				in := []string{"foo", "bar", "baz"}
				_, err := cliui.DisplayTable(in, "", nil)
				require.Error(t, err)
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
				_, err := cliui.DisplayTable(in, "", nil)
				require.Error(t, err)
			})

			t.Run("WithData", func(t *testing.T) {
				t.Parallel()

				in := []noTableTagsTest{{Field: "hi"}}
				_, err := cliui.DisplayTable(in, "", nil)
				require.Error(t, err)
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
				_, err := cliui.DisplayTable(in, "", nil)
				require.Error(t, err)
			})

			t.Run("WithData", func(t *testing.T) {
				t.Parallel()

				in := []noNameTest{{Field: "test"}}
				_, err := cliui.DisplayTable(in, "", nil)
				require.Error(t, err)
			})
		})

		t.Run("InvalidTag/BadSyntax", func(t *testing.T) {
			t.Parallel()

			type invalidSyntaxTest struct {
				Field string `table:"asda,asdjada"`
			}

			t.Run("WithoutData", func(t *testing.T) {
				t.Parallel()

				var in []invalidSyntaxTest
				_, err := cliui.DisplayTable(in, "", nil)
				require.Error(t, err)
			})

			t.Run("WithData", func(t *testing.T) {
				t.Parallel()

				in := []invalidSyntaxTest{{Field: "test"}}
				_, err := cliui.DisplayTable(in, "", nil)
				require.Error(t, err)
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
