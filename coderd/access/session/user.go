package session

import "github.com/coder/coder/coderd/database"

type userActor struct {
	user *database.User
}

var _ UserActor = &userActor{}

func NewUserActor(u *database.User) UserActor {
	return &userActor{
		user: u,
	}
}

func (*userActor) Type() ActorType {
	return ActorTypeUser
}

func (ua *userActor) ID() string {
	return ua.user.ID.String()
}

func (ua *userActor) Name() string {
	return ua.user.Username
}

func (ua *userActor) User() *database.User {
	return ua.user
}
