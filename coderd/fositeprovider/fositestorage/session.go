package fositestorage

import (
	"time"

	"github.com/google/uuid"
	"github.com/mohae/deepcopy"
	"github.com/ory/fosite"
)

var _ fosite.Session = (*CoderSession)(nil)

type CoderSession struct {
	ID uuid.UUID

	ExpiresAt map[fosite.TokenType]time.Time
	Requested time.Time
	UserID    uuid.UUID
	Username  string
}

func NewSession() *CoderSession {
	return &CoderSession{
		ID:        uuid.New(),
		Requested: time.Now().UTC(),
		ExpiresAt: nil,
	}
}

func (c CoderSession) SetExpiresAt(key fosite.TokenType, exp time.Time) {
	if c.ExpiresAt == nil {
		c.ExpiresAt = make(map[fosite.TokenType]time.Time)
	}
	c.ExpiresAt[key] = exp
}

func (c CoderSession) GetExpiresAt(key fosite.TokenType) time.Time {
	if c.ExpiresAt == nil {
		c.ExpiresAt = make(map[fosite.TokenType]time.Time)
	}

	if _, ok := c.ExpiresAt[key]; !ok {
		return time.Time{}
	}
	return c.ExpiresAt[key]
}

func (c CoderSession) GetUsername() string {
	return c.Username
}

func (c CoderSession) GetSubject() string {
	return c.UserID.String()
}

func (c CoderSession) Clone() fosite.Session {
	return deepcopy.Copy(c).(fosite.Session)
}
