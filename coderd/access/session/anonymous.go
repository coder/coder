package session

type anonymousActor struct{}

// Anon is a static AnonymousActor implementation.
var Anon AnonymousActor = anonymousActor{}

func (anonymousActor) Type() ActorType {
	return ActorTypeAnonymous
}

func (anonymousActor) ID() string {
	return "anon"
}

func (anonymousActor) Name() string {
	return "anonymous"
}

func (anonymousActor) Anonymous() {}
