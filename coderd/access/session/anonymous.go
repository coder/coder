package session

const AnonymousUserID = "anonymous"

type anonymousActor struct{}

// Anon is a static AnonymousActor implementation.
var Anon AnonymousActor = anonymousActor{}

func (anonymousActor) Type() ActorType {
	return ActorTypeAnonymous
}

func (anonymousActor) ID() string {
	return AnonymousUserID
}

func (anonymousActor) Name() string {
	return AnonymousUserID
}

func (anonymousActor) Anonymous() {}
