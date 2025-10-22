package proto

func (p PrebuiltWorkspaceBuildStage) IsPrebuild() bool {
	return p == PrebuiltWorkspaceBuildStage_CREATE
}

func (p PrebuiltWorkspaceBuildStage) IsPrebuiltWorkspaceClaim() bool {
	return p == PrebuiltWorkspaceBuildStage_CLAIM
}
