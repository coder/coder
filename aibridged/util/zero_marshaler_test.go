package util

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type TestStruct struct {
	Name    string            `json:"name"`
	Age     int               `json:"age"`
	Profile *ProfileStruct    `json:"profile"`
	Items   []string          `json:"items"`
	Meta    map[string]string `json:"meta"`
	Active  bool              `json:"active"`
	Score   float64           `json:"score"`
}

type ProfileStruct struct {
	Bio     string `json:"bio"`
	Website string `json:"website"`
}

func TestMarshalNoZero_BasicFunctionality(t *testing.T) {
	t.Parallel()
	// Test basic zero omission without exclusions
	data := TestStruct{
		Name: "John",
		// Age: 0 (zero value, should be omitted)
		// Profile: nil (zero value, should be omitted)
		Items: []string{}, // empty slice (zero value, should be omitted)
		// Meta: nil (zero value, should be omitted)
		// Active: false (zero value, should be omitted)
		// Score: 0.0 (zero value, should be omitted)
	}

	result, err := MarshalNoZero(data)
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	err = json.Unmarshal(result, &unmarshaled)
	require.NoError(t, err)

	// Should only contain non-zero fields
	require.Equal(t, "John", unmarshaled["name"])
	require.NotContains(t, unmarshaled, "age")
	require.NotContains(t, unmarshaled, "profile")
	require.NotContains(t, unmarshaled, "items")
	require.NotContains(t, unmarshaled, "meta")
	require.NotContains(t, unmarshaled, "active")
	require.NotContains(t, unmarshaled, "score")
}

func TestMarshalNoZero_WithNoOmitTag(t *testing.T) {
	t.Parallel()
	type TestStructWithTag struct {
		Name string `json:"name"`
		Age  int    `json:"age,no_omit"`
	}

	data := TestStructWithTag{
		Name: "John",
		Age:  0, // Should be included due to no_omit tag
	}

	result, err := MarshalNoZero(data)
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	err = json.Unmarshal(result, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, "John", unmarshaled["name"])
	require.Equal(t, float64(0), unmarshaled["age"]) // JSON numbers are float64
}

func TestMarshalNoZero_WithJMESPathExclusions(t *testing.T) {
	t.Parallel()
	data := TestStruct{
		Name:   "John",
		Age:    0,     // Should be included due to exclusion
		Score:  0.0,   // Should be included due to exclusion
		Active: false, // Should be included due to exclusion
		Profile: &ProfileStruct{
			Bio:     "", // Should be included due to exclusion
			Website: "", // Should be omitted (not in exclusion)
		},
		Items: []string{},          // Should be included due to exclusion
		Meta:  map[string]string{}, // Should be omitted (not in exclusion)
	}

	result, err := MarshalNoZero(data, "age", "score", "active", "profile.bio", "items")
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	err = json.Unmarshal(result, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, "John", unmarshaled["name"])
	require.Equal(t, float64(0), unmarshaled["age"])
	require.Equal(t, float64(0), unmarshaled["score"])
	require.Equal(t, false, unmarshaled["active"])
	require.Contains(t, unmarshaled, "items")
	require.NotContains(t, unmarshaled, "meta")

	// Check nested profile
	profile := unmarshaled["profile"].(map[string]interface{})
	require.Equal(t, "", profile["bio"])
	require.NotContains(t, profile, "website")
}

func TestMarshalNoZero_ArrayExclusions(t *testing.T) {
	t.Parallel()
	type ArrayTest struct {
		Items []TestStruct `json:"items"`
	}

	data := ArrayTest{
		Items: []TestStruct{
			{Name: "John", Age: 0}, // Age should be omitted normally
			{Name: "", Age: 25},    // Name should be omitted normally
		},
	}

	// Test excluding specific array elements
	result, err := MarshalNoZero(data, "items[0].age", "items[1].name")
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	err = json.Unmarshal(result, &unmarshaled)
	require.NoError(t, err)

	items := unmarshaled["items"].([]interface{})
	require.Len(t, items, 2)

	// First item should have age included
	item0 := items[0].(map[string]interface{})
	require.Equal(t, "John", item0["name"])
	require.Equal(t, float64(0), item0["age"])

	// Second item should have name included
	item1 := items[1].(map[string]interface{})
	require.Equal(t, "", item1["name"])
	require.Equal(t, float64(25), item1["age"])
}

func TestMarshalNoZero_WildcardExclusions(t *testing.T) {
	t.Parallel()
	type WildcardTest struct {
		Users []TestStruct `json:"users"`
	}

	data := WildcardTest{
		Users: []TestStruct{
			{Name: "John", Age: 0},
			{Name: "Jane", Age: 0},
		},
	}

	// Test wildcard exclusion
	result, err := MarshalNoZero(data, "users[*].age")
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	err = json.Unmarshal(result, &unmarshaled)
	require.NoError(t, err)

	users := unmarshaled["users"].([]interface{})
	require.Len(t, users, 2)

	// Both users should have age included
	user0 := users[0].(map[string]interface{})
	require.Equal(t, "John", user0["name"])
	require.Equal(t, float64(0), user0["age"])

	user1 := users[1].(map[string]interface{})
	require.Equal(t, "Jane", user1["name"])
	require.Equal(t, float64(0), user1["age"])
}

func TestMarshalNoZero_MapExclusions(t *testing.T) {
	t.Parallel()
	data := TestStruct{
		Name: "John",
		Meta: map[string]string{
			"key1": "", // Should be omitted normally
			"key2": "value",
		},
	}

	result, err := MarshalNoZero(data, "meta.key1")
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	err = json.Unmarshal(result, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, "John", unmarshaled["name"])
	meta := unmarshaled["meta"].(map[string]interface{})
	require.Equal(t, "", meta["key1"]) // Should be included due to exclusion
	require.Equal(t, "value", meta["key2"])
}

func TestMarshalNoZero_ComplexNestedExclusions(t *testing.T) {
	t.Parallel()
	type NestedTest struct {
		Level1 struct {
			Level2 struct {
				Value string `json:"value"`
				Count int    `json:"count"`
			} `json:"level2"`
			Items []struct {
				Name string `json:"name"`
				ID   int    `json:"id"`
			} `json:"items"`
		} `json:"level1"`
	}

	data := NestedTest{}
	data.Level1.Level2.Value = "" // Should be included due to exclusion
	data.Level1.Level2.Count = 0  // Should be omitted
	data.Level1.Items = []struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	}{
		{Name: "", ID: 0}, // Name should be included due to exclusion
	}

	result, err := MarshalNoZero(data, "level1.level2.value", "level1.items[0].name")
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	err = json.Unmarshal(result, &unmarshaled)
	require.NoError(t, err)

	level1 := unmarshaled["level1"].(map[string]interface{})
	level2 := level1["level2"].(map[string]interface{})
	require.Equal(t, "", level2["value"])
	require.NotContains(t, level2, "count")

	items := level1["items"].([]interface{})
	require.Len(t, items, 1)
	item0 := items[0].(map[string]interface{})
	require.Equal(t, "", item0["name"])
	require.NotContains(t, item0, "id")
}

func TestMarshalNoZero_EmptyExclusions(t *testing.T) {
	t.Parallel()
	data := TestStruct{
		Name: "John",
		Age:  0,
	}

	// Test with empty exclusions array
	result, err := MarshalNoZero(data, []string{}...)
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	err = json.Unmarshal(result, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, "John", unmarshaled["name"])
	require.NotContains(t, unmarshaled, "age")
}

func TestMarshalNoZero_InvalidJMESPath(t *testing.T) {
	t.Parallel()
	data := TestStruct{
		Name: "John",
		Age:  0,
	}

	// Test with invalid JMESPath - should not crash, just ignore invalid patterns
	result, err := MarshalNoZero(data, "invalid[[[path", "age")
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	err = json.Unmarshal(result, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, "John", unmarshaled["name"])
	require.Equal(t, float64(0), unmarshaled["age"]) // Should still be included due to valid "age" exclusion
}

func TestMarshalNoZero_BackwardCompatibility(t *testing.T) {
	t.Parallel()
	data := TestStruct{
		Name: "John",
		Age:  0,
	}

	// Test calling without any exclusions (backward compatibility)
	result, err := MarshalNoZero(data)
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	err = json.Unmarshal(result, &unmarshaled)
	require.NoError(t, err)

	require.Equal(t, "John", unmarshaled["name"])
	require.NotContains(t, unmarshaled, "age")
}

func TestCreateTestObject(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path     string
		expected map[string]interface{}
		hasError bool
	}{
		{
			path: "user.name",
			expected: map[string]interface{}{
				"user": map[string]interface{}{
					"name": true,
				},
			},
			hasError: false,
		},
		{
			path: "items[0].name",
			expected: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"name": true,
					},
				},
			},
			hasError: false,
		},
		{
			path: "simple",
			expected: map[string]interface{}{
				"simple": true,
			},
			hasError: false,
		},
		{
			path:     "",
			expected: nil,
			hasError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()
			result, err := createTestObject(test.path)
			if test.hasError {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, result)
			}
		})
	}
}

func TestMatchesExclusion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path       string
		exclusions []string
		expected   bool
	}{
		{"user.name", []string{"user.name"}, true},
		{"user.age", []string{"user.name"}, false},
		{"items[0].name", []string{"items[0].name"}, true},
		{"items[1].name", []string{"items[0].name"}, false},
		{"user.profile.bio", []string{"user.profile.*"}, true},
		{"", []string{"user.name"}, false},
		{"user.name", []string{}, false},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()
			result := matchesExclusion(test.path, test.exclusions)
			require.Equal(t, test.expected, result)
		})
	}
}
