package dataprotection_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/dataprotection"
	"github.com/coder/coder/v2/codersdk"
)

func TestConfig_ShouldObfuscate(t *testing.T) {
	t.Parallel()

	t.Run("DisabledReturnsFalse", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfig(0, nil, 5)
		assert.False(t, cfg.ShouldObfuscate("anyone@example.com"))
	})

	t.Run("NilConfigReturnsFalse", func(t *testing.T) {
		t.Parallel()
		var cfg *dataprotection.Config
		assert.False(t, cfg.ShouldObfuscate("anyone@example.com"))
	})

	t.Run("EnabledAuditorReturnsFalse", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfig(1, []string{"auditor@example.com"}, 5)
		assert.False(t, cfg.ShouldObfuscate("auditor@example.com"))
	})

	t.Run("EnabledNonAuditorReturnsTrue", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfig(1, []string{"auditor@example.com"}, 5)
		assert.True(t, cfg.ShouldObfuscate("manager@example.com"))
	})

	t.Run("EnabledEmptyAuditorsReturnsTrue", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfig(1, nil, 5)
		assert.True(t, cfg.ShouldObfuscate("anyone@example.com"))
	})
}

func TestConfig_IsAuditor(t *testing.T) {
	t.Parallel()

	t.Run("DisabledAlwaysFalse", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfig(0, []string{"a@co.de"}, 5)
		assert.False(t, cfg.IsAuditor("a@co.de"))
	})

	t.Run("EnabledMatchesEmail", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfig(1, []string{"a@co.de", "b@co.de"}, 5)
		assert.True(t, cfg.IsAuditor("a@co.de"))
		assert.True(t, cfg.IsAuditor("b@co.de"))
		assert.False(t, cfg.IsAuditor("c@co.de"))
	})
}

func TestConfig_ObfuscateUserID(t *testing.T) {
	t.Parallel()

	fixedKey := []byte("test-key-for-deterministic-tests")
	cfg := dataprotection.NewConfigForTest(1, nil, 5, fixedKey)

	realID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	t.Run("ReturnsDifferentUUID", func(t *testing.T) {
		t.Parallel()
		pseudoID := cfg.ObfuscateUserID(realID)
		assert.NotEqual(t, realID, pseudoID)
	})

	t.Run("Deterministic", func(t *testing.T) {
		t.Parallel()
		a := cfg.ObfuscateUserID(realID)
		b := cfg.ObfuscateUserID(realID)
		assert.Equal(t, a, b)
	})

	t.Run("DifferentInputsDifferentOutputs", func(t *testing.T) {
		t.Parallel()
		id2 := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		a := cfg.ObfuscateUserID(realID)
		b := cfg.ObfuscateUserID(id2)
		assert.NotEqual(t, a, b)
	})

	t.Run("ValidUUIDv4", func(t *testing.T) {
		t.Parallel()
		pseudoID := cfg.ObfuscateUserID(realID)
		// Version 4 bits.
		assert.Equal(t, byte(0x40), pseudoID[6]&0xf0)
		// Variant 1 bits.
		assert.Equal(t, byte(0x80), pseudoID[8]&0xc0)
	})
}

func TestConfig_ObfuscateUserActivities(t *testing.T) {
	t.Parallel()

	fixedKey := []byte("test-key-for-deterministic-tests")

	t.Run("ObfuscatesFields", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfigForTest(1, nil, 2, fixedKey)

		users := []codersdk.UserActivity{
			{
				UserID:    uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001"),
				Username:  "alice",
				AvatarURL: "https://example.com/alice.png",
				Seconds:   100,
			},
			{
				UserID:    uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000002"),
				Username:  "bob",
				AvatarURL: "https://example.com/bob.png",
				Seconds:   200,
			},
			{
				UserID:    uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000003"),
				Username:  "charlie",
				AvatarURL: "https://example.com/charlie.png",
				Seconds:   300,
			},
		}

		result := cfg.ObfuscateUserActivities(users)
		require.Len(t, result, 3)

		for i, u := range result {
			// Identity fields are replaced.
			assert.NotEqual(t, users[i].UserID, u.UserID, "user_id should be pseudonymized")
			assert.NotEqual(t, users[i].Username, u.Username, "username should be pseudonymized")
			assert.Empty(t, u.AvatarURL, "avatar_url should be empty")

			// Pseudonym is deterministic.
			expectedPID := cfg.ObfuscateUserID(users[i].UserID)
			assert.Equal(t, expectedPID, u.UserID)
			assert.Equal(t, dataprotection.PseudoUsername(expectedPID), u.Username)

			// Data fields are preserved.
			assert.Equal(t, users[i].Seconds, u.Seconds)
			assert.Equal(t, users[i].TemplateIDs, u.TemplateIDs)
		}
	})

	t.Run("EmptySlice", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfigForTest(1, nil, 0, fixedKey)
		result := cfg.ObfuscateUserActivities([]codersdk.UserActivity{})
		require.Empty(t, result)
	})

	t.Run("SameUserSamePseudonym", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfigForTest(1, nil, 1, fixedKey)

		uid := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
		users := []codersdk.UserActivity{
			{UserID: uid, Username: "alice", Seconds: 100},
		}
		result := cfg.ObfuscateUserActivities(users)
		require.Len(t, result, 1)

		// Call again with the same user — should get same pseudonym.
		result2 := cfg.ObfuscateUserActivities(users)
		require.Len(t, result2, 1)
		assert.Equal(t, result[0].UserID, result2[0].UserID)
		assert.Equal(t, result[0].Username, result2[0].Username)
	})
}

func TestConfig_ObfuscateUserLatencies(t *testing.T) {
	t.Parallel()

	fixedKey := []byte("test-key-for-deterministic-tests")
	cfg := dataprotection.NewConfigForTest(1, nil, 2, fixedKey)

	users := []codersdk.UserLatency{
		{
			UserID:    uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001"),
			Username:  "alice",
			AvatarURL: "https://example.com/alice.png",
			LatencyMS: codersdk.ConnectionLatency{P50: 10.0, P95: 50.0},
		},
		{
			UserID:    uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000002"),
			Username:  "bob",
			AvatarURL: "https://example.com/bob.png",
			LatencyMS: codersdk.ConnectionLatency{P50: 20.0, P95: 100.0},
		},
	}

	result := cfg.ObfuscateUserLatencies(users)
	require.Len(t, result, 2)

	for i, u := range result {
		assert.NotEqual(t, users[i].UserID, u.UserID)
		assert.NotEqual(t, users[i].Username, u.Username)
		assert.Empty(t, u.AvatarURL)
		// Latency data preserved.
		assert.Equal(t, users[i].LatencyMS, u.LatencyMS)
	}
}

func TestConfig_SuppressSmallGroups(t *testing.T) {
	t.Parallel()

	fixedKey := []byte("test-key-for-deterministic-tests")

	t.Run("BelowThresholdSuppressed", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfigForTest(1, nil, 5, fixedKey)

		users := make([]codersdk.UserActivity, 4)
		for i := range users {
			users[i] = codersdk.UserActivity{
				UserID:   uuid.New(),
				Username: "user",
				Seconds:  int64(i * 100),
			}
		}
		result := cfg.ObfuscateUserActivities(users)
		require.Empty(t, result, "4 users with min_group_size=5 should be suppressed")
	})

	t.Run("AtThresholdNotSuppressed", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfigForTest(1, nil, 5, fixedKey)

		users := make([]codersdk.UserActivity, 5)
		for i := range users {
			users[i] = codersdk.UserActivity{
				UserID:   uuid.New(),
				Username: "user",
				Seconds:  int64(i * 100),
			}
		}
		result := cfg.ObfuscateUserActivities(users)
		require.Len(t, result, 5)
	})

	t.Run("AboveThresholdNotSuppressed", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfigForTest(1, nil, 5, fixedKey)

		users := make([]codersdk.UserActivity, 6)
		for i := range users {
			users[i] = codersdk.UserActivity{
				UserID:   uuid.New(),
				Username: "user",
				Seconds:  int64(i * 100),
			}
		}
		result := cfg.ObfuscateUserActivities(users)
		require.Len(t, result, 6)
	})

	t.Run("EmptySliceSuppressed", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfigForTest(1, nil, 5, fixedKey)
		result := cfg.ObfuscateUserActivities(nil)
		require.Empty(t, result)
	})
}

func TestConfig_CrossEndpointConsistency(t *testing.T) {
	t.Parallel()

	fixedKey := []byte("test-key-for-deterministic-tests")
	cfg := dataprotection.NewConfigForTest(1, nil, 1, fixedKey)

	uid := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")

	activities := []codersdk.UserActivity{
		{UserID: uid, Username: "alice", Seconds: 100},
	}
	latencies := []codersdk.UserLatency{
		{UserID: uid, Username: "alice", LatencyMS: codersdk.ConnectionLatency{P50: 10}},
	}

	actResult := cfg.ObfuscateUserActivities(activities)
	latResult := cfg.ObfuscateUserLatencies(latencies)

	require.Len(t, actResult, 1)
	require.Len(t, latResult, 1)

	// Same real user_id should produce the same pseudonym across
	// different obfuscation functions.
	assert.Equal(t, actResult[0].UserID, latResult[0].UserID,
		"pseudonym UUID should be consistent across endpoints")
	assert.Equal(t, actResult[0].Username, latResult[0].Username,
		"pseudonym username should be consistent across endpoints")
}

func TestConfig_ObfuscateChatCostUsers(t *testing.T) {
	t.Parallel()

	fixedKey := []byte("test-key-for-deterministic-tests")
	cfg := dataprotection.NewConfigForTest(1, nil, 1, fixedKey)

	users := []codersdk.ChatCostUserRollup{
		{
			UserID:          uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001"),
			Username:        "alice",
			Name:            "Alice Smith",
			AvatarURL:       "https://example.com/alice.png",
			TotalCostMicros: 5000,
			MessageCount:    10,
			ChatCount:       2,
		},
	}

	result := cfg.ObfuscateChatCostUsers(users)
	require.Len(t, result, 1)

	u := result[0]
	assert.NotEqual(t, users[0].UserID, u.UserID)
	assert.NotEqual(t, users[0].Username, u.Username)
	assert.Empty(t, u.Name, "display name should be empty")
	assert.Empty(t, u.AvatarURL, "avatar_url should be empty")
	// Data preserved.
	assert.Equal(t, int64(5000), u.TotalCostMicros)
	assert.Equal(t, int64(10), u.MessageCount)
	assert.Equal(t, int64(2), u.ChatCount)
}

func TestConfig_Tiers(t *testing.T) {
	t.Parallel()

	t.Run("Tier0IsOff", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfig(0, nil, 5)
		assert.False(t, cfg.IsTier1OrAbove())
		assert.False(t, cfg.IsTier2OrAbove())
		assert.False(t, cfg.ShouldObfuscate("anyone@example.com"))
	})

	t.Run("Tier1", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfig(1, []string{"auditor@co.de"}, 5)
		assert.True(t, cfg.IsTier1OrAbove())
		assert.False(t, cfg.IsTier2OrAbove())
		assert.True(t, cfg.ShouldObfuscate("manager@co.de"))
		assert.False(t, cfg.ShouldObfuscate("auditor@co.de"))
	})

	t.Run("Tier2", func(t *testing.T) {
		t.Parallel()
		cfg := dataprotection.NewConfig(2, []string{"auditor@co.de"}, 5)
		assert.True(t, cfg.IsTier1OrAbove())
		assert.True(t, cfg.IsTier2OrAbove())
	})
}

func TestConfig_PseudoSlug(t *testing.T) {
	t.Parallel()

	fixedKey := []byte("test-key-for-deterministic-tests")
	cfg := dataprotection.NewConfigForTest(2, nil, 5, fixedKey)

	uid := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	slug := cfg.PseudoSlug(uid)

	t.Run("URLSafe", func(t *testing.T) {
		t.Parallel()
		assert.Contains(t, slug, "protected-")
		assert.NotContains(t, slug, " ")
	})

	t.Run("Deterministic", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, slug, cfg.PseudoSlug(uid))
	})

	t.Run("ResolveSlug", func(t *testing.T) {
		t.Parallel()
		resolved, ok := cfg.ResolveSlug(slug)
		assert.True(t, ok)
		assert.Equal(t, uid, resolved)
	})

	t.Run("UnknownSlugFails", func(t *testing.T) {
		t.Parallel()
		_, ok := cfg.ResolveSlug("protected-00000000")
		assert.False(t, ok)
	})
}

func TestConfig_ObfuscateMinimalUser(t *testing.T) {
	t.Parallel()

	fixedKey := []byte("test-key-for-deterministic-tests")
	cfg := dataprotection.NewConfigForTest(2, nil, 5, fixedKey)

	selfID := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	otherID := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000002")

	t.Run("ObfuscatesOther", func(t *testing.T) {
		t.Parallel()
		u := codersdk.MinimalUser{ID: otherID, Username: "bob", Name: "Bob", AvatarURL: "http://img"}
		cfg.ObfuscateMinimalUser(&u, selfID)
		assert.NotEqual(t, otherID, u.ID)
		assert.Contains(t, u.Username, "Protected User")
		assert.Empty(t, u.Name)
		assert.Empty(t, u.AvatarURL)
	})

	t.Run("SkipsSelf", func(t *testing.T) {
		t.Parallel()
		u := codersdk.MinimalUser{ID: selfID, Username: "alice", Name: "Alice", AvatarURL: "http://img"}
		cfg.ObfuscateMinimalUser(&u, selfID)
		assert.Equal(t, selfID, u.ID)
		assert.Equal(t, "alice", u.Username)
		assert.Equal(t, "Alice", u.Name)
	})
}
