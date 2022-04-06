package session

type systemActor struct{}

// System is a static SystemActor implementation.
var System SystemActor = systemActor{}

func (systemActor) Type() ActorType {
	return ActorTypeSystem
}

func (systemActor) ID() string {
	return "system"
}

func (systemActor) Name() string {
	return "system"
}

func (systemActor) System() {}
