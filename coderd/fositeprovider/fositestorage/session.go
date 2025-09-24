package fositestorage

import (
	"time"

	"github.com/google/uuid"
	"github.com/ory/fosite"
)

var _ fosite.Session = (*CoderSession)(nil)

// TODO: Make a custom session that includes user ID and other relevant info.
type CoderSession struct {
	UserID uuid.UUID `json:"user_id"`

	//Claims    *jwt.IDTokenClaims             `json:"id_token_claims"`
	//Headers   *jwt.Headers                   `json:"headers"`
	//ExpiresAt map[fosite.TokenType]time.Time `json:"expires_at"`
	//Username  string                         `json:"username,omitempty"`
	//Subject   string                         `json:"subject,omitempty"`
}

func (c CoderSession) SetExpiresAt(key fosite.TokenType, exp time.Time) {
	//TODO implement me
	panic("implement me")
}

func (c CoderSession) GetExpiresAt(key fosite.TokenType) time.Time {
	//TODO implement me
	panic("implement me")
}

func (c CoderSession) GetUsername() string {
	//TODO implement me
	panic("implement me")
}

func (c CoderSession) GetSubject() string {
	//TODO implement me
	panic("implement me")
}

func (c CoderSession) Clone() fosite.Session {
	//TODO implement me
	panic("implement me")
}

func (c CoderSession) SetSubject(subject string) {
	//TODO implement me
	panic("implement me")
}
