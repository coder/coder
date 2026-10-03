package chatdebug

// AllRunKinds contains every RunKind value for SDK parity checks.
var AllRunKinds = []RunKind{
	KindChatTurn,
	KindTitleGeneration,
	KindQuickgen,
	KindCompaction,
}

// AllStatuses contains every Status value for SDK parity checks.
var AllStatuses = []Status{
	StatusInProgress,
	StatusCompleted,
	StatusError,
	StatusInterrupted,
}

// AllOperations contains every Operation value for SDK parity checks.
var AllOperations = []Operation{
	OperationStream,
	OperationGenerate,
}
