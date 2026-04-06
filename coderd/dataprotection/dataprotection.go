// Package dataprotection implements Data Protection Mode (DPM) for
// compliance with employee data protection regulations. When enabled,
// it obfuscates individual user identifiers in reports and analytics
// while preserving aggregate statistics.
package dataprotection

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

// Config holds runtime-resolved DPM configuration. It is created once
// at server startup and is immutable for the lifetime of the process.
type Config struct {
	Enabled      bool
	Auditors     map[string]bool // email → true
	MinGroupSize int
	instanceKey  []byte // random per-startup key for pseudonym generation
}

// NewConfig creates a DPM config. A random instance key is generated
// for deterministic pseudonym generation that rotates on server
// restart.
func NewConfig(enabled bool, auditors []string, minGroupSize int) *Config {
	key := make([]byte, 32)
	_, _ = rand.Read(key)

	auditorMap := make(map[string]bool, len(auditors))
	for _, a := range auditors {
		auditorMap[a] = true
	}
	if minGroupSize <= 0 {
		minGroupSize = 5
	}
	return &Config{
		Enabled:      enabled,
		Auditors:     auditorMap,
		MinGroupSize: minGroupSize,
		instanceKey:  key,
	}
}

// NewConfigForTest creates a DPM config with a fixed instance key for
// deterministic testing. Do not use in production.
func NewConfigForTest(enabled bool, auditors []string, minGroupSize int, instanceKey []byte) *Config {
	cfg := NewConfig(enabled, auditors, minGroupSize)
	cfg.instanceKey = instanceKey
	return cfg
}

// IsAuditor reports whether the given email belongs to a designated
// DPM auditor.
func (c *Config) IsAuditor(email string) bool {
	if c == nil || !c.Enabled {
		return false
	}
	return c.Auditors[email]
}

// ShouldObfuscate reports whether data should be obfuscated for a
// user with the given email. Returns false when DPM is disabled or
// the user is an auditor.
func (c *Config) ShouldObfuscate(email string) bool {
	if c == nil || !c.Enabled {
		return false
	}
	return !c.Auditors[email]
}

// ObfuscateUserID returns a deterministic pseudonymous UUID for the
// given real user ID. The same real ID always maps to the same
// pseudonym within a server lifecycle because the HMAC key is fixed
// per startup.
func (c *Config) ObfuscateUserID(realID uuid.UUID) uuid.UUID {
	mac := hmac.New(sha256.New, c.instanceKey)
	_, _ = mac.Write(realID[:])
	sum := mac.Sum(nil)
	pseudoID, _ := uuid.FromBytes(sum[:16])
	// Set UUID v4 variant bits so the result is a valid UUID.
	pseudoID[6] = (pseudoID[6] & 0x0f) | 0x40
	pseudoID[8] = (pseudoID[8] & 0x3f) | 0x80
	return pseudoID
}

// PseudoUsername returns a deterministic pseudonymous username derived
// from a pseudonym UUID: "User <first 8 hex chars>".
func PseudoUsername(pseudoID uuid.UUID) string {
	return fmt.Sprintf("User %s", pseudoID.String()[:8])
}

// ObfuscateUserActivities replaces user identity fields with
// deterministic pseudonyms. Returns nil if the group size is below
// MinGroupSize (suppression).
func (c *Config) ObfuscateUserActivities(users []codersdk.UserActivity) []codersdk.UserActivity {
	if len(users) < c.MinGroupSize {
		return nil
	}
	result := make([]codersdk.UserActivity, len(users))
	for i, u := range users {
		pid := c.ObfuscateUserID(u.UserID)
		result[i] = codersdk.UserActivity{
			TemplateIDs: u.TemplateIDs,
			UserID:      pid,
			Username:    PseudoUsername(pid),
			AvatarURL:   "",
			Seconds:     u.Seconds,
		}
	}
	return result
}

// ObfuscateUserLatencies replaces user identity fields with
// deterministic pseudonyms. Returns nil if the group size is below
// MinGroupSize (suppression).
func (c *Config) ObfuscateUserLatencies(users []codersdk.UserLatency) []codersdk.UserLatency {
	if len(users) < c.MinGroupSize {
		return nil
	}
	result := make([]codersdk.UserLatency, len(users))
	for i, u := range users {
		pid := c.ObfuscateUserID(u.UserID)
		result[i] = codersdk.UserLatency{
			TemplateIDs: u.TemplateIDs,
			UserID:      pid,
			Username:    PseudoUsername(pid),
			AvatarURL:   "",
			LatencyMS:   u.LatencyMS,
		}
	}
	return result
}

// ObfuscateChatCostUsers replaces user identity fields with
// deterministic pseudonyms. Returns nil if the group size is below
// MinGroupSize (suppression).
func (c *Config) ObfuscateChatCostUsers(users []codersdk.ChatCostUserRollup) []codersdk.ChatCostUserRollup {
	if len(users) < c.MinGroupSize {
		return nil
	}
	result := make([]codersdk.ChatCostUserRollup, len(users))
	for i, u := range users {
		pid := c.ObfuscateUserID(u.UserID)
		result[i] = codersdk.ChatCostUserRollup{
			UserID:                   pid,
			Username:                 PseudoUsername(pid),
			Name:                     "",
			AvatarURL:                "",
			TotalCostMicros:          u.TotalCostMicros,
			MessageCount:             u.MessageCount,
			ChatCount:                u.ChatCount,
			TotalInputTokens:         u.TotalInputTokens,
			TotalOutputTokens:        u.TotalOutputTokens,
			TotalCacheReadTokens:     u.TotalCacheReadTokens,
			TotalCacheCreationTokens: u.TotalCacheCreationTokens,
		}
	}
	return result
}
