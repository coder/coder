package chatexec

import "golang.org/x/xerrors"

// ErrRequiresAction indicates the chatloop paused so the caller can
// fulfill a dynamic tool call before execution resumes.
var ErrRequiresAction = xerrors.New("requires action")
