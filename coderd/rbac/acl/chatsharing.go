package acl

import (
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// ChatSharingDisabled reports whether chat ACL grants are ignored. It also
// checks the deployment option, because callers such as telemetry can run
// before coderd applies it with rbac.SetChatACLDisabled.
func ChatSharingDisabled(dv *codersdk.DeploymentValues) bool {
	return rbac.ChatACLDisabled() || (dv != nil && bool(dv.DisableChatSharing))
}
