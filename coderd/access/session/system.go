package session

const SystemUserID = "system"

type systemActor struct{}

// System is a static SystemActor implementation.
var System SystemActor = systemActor{}

func (systemActor) Type() ActorType {
	return ActorTypeSystem
}

func (systemActor) ID() string {
	return SystemUserID
}

func (systemActor) Name() string {
	return SystemUserID
}

func (systemActor) System() {}
