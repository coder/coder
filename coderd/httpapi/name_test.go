package httpapi_test

import (
	"testing"

	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
)

func TestUsernameValid(t *testing.T) {
	t.Parallel()
	// Tests whether usernames are valid or not.
	testCases := []struct {
		Username string
		Valid    bool
	}{
		{"1", true},
		{"12", true},
		{"123", true},
		{"12345678901234567890", true},
		{"123456789012345678901", true},
		{"a", true},
		{"a1", true},
		{"a1b2", true},
		{"a1b2c3d4e5f6g7h8i9j0", true},
		{"a1b2c3d4e5f6g7h8i9j0k", true},
		{"aa", true},
		{"abc", true},
		{"abcdefghijklmnopqrst", true},
		{"abcdefghijklmnopqrstu", true},
		{"wow-test", true},

		{"", false},
		{" ", false},
		{" a", false},
		{" a ", false},
		{" 1", false},
		{"1 ", false},
		{" aa", false},
		{"aa ", false},
		{" 12", false},
		{"12 ", false},
		{" a1", false},
		{"a1 ", false},
		{" abcdefghijklmnopqrstu", false},
		{"abcdefghijklmnopqrstu ", false},
		{" 123456789012345678901", false},
		{" a1b2c3d4e5f6g7h8i9j0k", false},
		{"a1b2c3d4e5f6g7h8i9j0k ", false},
		{"bananas_wow", false},
		{"test--now", false},

		{"123456789012345678901234567890123", false},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"123456789012345678901234567890123123456789012345678901234567890123", false},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Username, func(t *testing.T) {
			t.Parallel()
			valid := httpapi.NameValid(testCase.Username)
			require.Equal(t, testCase.Valid, valid == nil)
		})
	}
}

func TestTemplateDisplayNameValid(t *testing.T) {
	t.Parallel()
	// Tests whether display names are valid.
	testCases := []struct {
		Name  string
		Valid bool
	}{
		{"", true},
		{"1", true},
		{"12", true},
		{"1 2", true},
		{"123       456", true},
		{"1234 678901234567890", true},
		{"<b> </b>", true},
		{"S", true},
		{"a1", true},
		{"a1K2", true},
		{"!!!!1 ?????", true},
		{"k\r\rm", true},
		{"abcdefghijklmnopqrst", true},
		{"Wow Test", true},
		{"abcdefghijklmnopqrstu-", true},
		{"a1b2c3d4e5f6g7h8i9j0k-", true},
		{"BANANAS_wow", true},
		{"test--now", true},
		{"123456789012345678901234567890123", true},
		{"1234567890123456789012345678901234567890123456789012345678901234", true},
		{"-a1b2c3d4e5f6g7h8i9j0k", true},

		{" ", false},
		{"\t", false},
		{"\r\r", false},
		{"\t1 ", false},
		{" a", false},
		{"\ra ", false},
		{" 1", false},
		{"1 ", false},
		{" aa", false},
		{"aa\r", false},
		{" 12", false},
		{"12 ", false},
		{"\fa1", false},
		{"a1\t", false},
		{"12345678901234567890123456789012345678901234567890123456789012345", false},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()
			valid := httpapi.TemplateDisplayNameValid(testCase.Name)
			require.Equal(t, testCase.Valid, valid == nil)
		})
	}
}

func TestTemplateVersionNameValid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name  string
		Valid bool
	}{
		{"1", true},
		{"12", true},
		{"1_2", true},
		{"1-2", true},
		{"cray", true},
		{"123_456", true},
		{"123-456", true},
		{"1234_678901234567890", true},
		{"1234-678901234567890", true},
		{"S", true},
		{"a1", true},
		{"a1K2", true},
		{"fuzzy_bear3", true},
		{"fuzzy-bear3", true},
		{"v1.0.0", true},
		{"heuristic_cray2", true},

		{"", false},
		{".v1", false},
		{"v1..0", false},
		{"4--4", false},
		{"<b> </b>", false},
		{"!!!!1 ?????", false},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()
			valid := httpapi.TemplateVersionNameValid(testCase.Name)
			require.Equal(t, testCase.Valid, valid == nil)
		})
	}
}

func TestGeneratedTemplateVersionNameValid(t *testing.T) {
	t.Parallel()

	for i := 0; i < 1000; i++ {
		name := namesgenerator.GetRandomName(1)
		err := httpapi.TemplateVersionNameValid(name)
		require.NoError(t, err, "invalid template version name: %s", name)
	}
}

func TestFrom(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		From  string
		Match string
	}{
		{"1", "1"},
		{"kyle@kwc.io", "kyle"},
		{"kyle+wow@kwc.io", "kylewow"},
		{"kyle+testing", "kyletesting"},
		{"kyle-testing", "kyle-testing"},
		{"much.”more unusual”@example.com", "muchmoreunusual"},

		// Cases where an invalid string is provided, and the result is a random name.
		{"123456789012345678901234567890123", ""},
		{"very.unusual.”@”.unusual.com@example.com", ""},
		{"___@ok.com", ""},
		{" something with spaces ", ""},
		{"--test--", ""},
		{"", ""},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.From, func(t *testing.T) {
			t.Parallel()
			converted := httpapi.UsernameFrom(testCase.From)
			t.Log(converted)
			valid := httpapi.NameValid(converted)
			require.True(t, valid == nil)
			if testCase.Match == "" {
				require.NotEqual(t, testCase.From, converted)
			} else {
				require.Equal(t, testCase.Match, converted)
			}
		})
	}
}

func TestUserRealNameValid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name  string
		Valid bool
	}{
		{"1", true},
		{"A", true},
		{"A1", true},
		{".", true},
		{"Mr Bean", true},
		{"Severus Snape", true},
		{"Prof. Albus Percival Wulfric Brian Dumbledore", true},
		{"Pablo Diego José Francisco de Paula Juan Nepomuceno María de los Remedios Cipriano de la Santísima Trinidad Ruiz y Picasso", true},
		{"Hector Ó hEochagáin", true},
		{"Małgorzata Kalinowska-Iszkowska", true},
		{"成龍", true},
		{". .", true},

		{"Lord Voldemort ", false},
		{" Bellatrix Lestrange", false},
		{" ", false},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()
			valid := httpapi.UserRealNameValid(testCase.Name)
			require.Equal(t, testCase.Valid, valid == nil)
		})
	}
}
