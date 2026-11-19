package codersdk

// PreviewMode is how a server hosts agent-authored previews.
type PreviewMode string

// PreviewModeStrict serves previews on the app wildcard hostname, the same
// site as the user's other apps, with a policy that keeps each preview on its
// own host.
const PreviewModeStrict PreviewMode = "strict"
