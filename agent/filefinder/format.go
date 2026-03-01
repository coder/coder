package filefinder

// FileFlag represents the type of filesystem entry.
type FileFlag uint16

const (
	FlagFile    FileFlag = 0
	FlagDir     FileFlag = 1
	FlagSymlink FileFlag = 2
)
