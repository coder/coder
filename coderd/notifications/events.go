package notifications

import "github.com/google/uuid"

// These vars are mapped to UUIDs in the notification_templates table.
// TODO: autogenerate these: https://github.com/coder/team-coconut/issues/36

// Workspace-related events.
var (
	TemplateWorkspaceCreated           = uuid.MustParse("281fdf73-c6d6-4cbb-8ff5-888baf8a2fff")
	TemplateWorkspaceManuallyUpdated   = uuid.MustParse("d089fe7b-d5c5-4c0c-aaf5-689859f7d392")
	TemplateWorkspaceDeleted           = uuid.MustParse("f517da0b-cdc9-410f-ab89-a86107c420ed")
	TemplateWorkspaceAutobuildFailed   = uuid.MustParse("381df2a9-c0c0-4749-420f-80a9280c66f9")
	TemplateWorkspaceDormant           = uuid.MustParse("0ea69165-ec14-4314-91f1-69566ac3c5a0")
	TemplateWorkspaceAutoUpdated       = uuid.MustParse("c34a0c09-0704-4cac-bd1c-0c0146811c2b")
	TemplateWorkspaceMarkedForDeletion = uuid.MustParse("51ce2fdf-c9ca-4be1-8d70-628674f9bc42")
	TemplateWorkspaceManualBuildFailed = uuid.MustParse("2faeee0f-26cb-4e96-821c-85ccb9f71513")
	TemplateWorkspaceOutOfMemory       = uuid.MustParse("a9d027b4-ac49-4fb1-9f6d-45af15f64e7a")
	TemplateWorkspaceOutOfDisk         = uuid.MustParse("f047f6a3-5713-40f7-85aa-0394cce9fa3a")
)

// Account-related events.
var (
	TemplateUserAccountCreated = uuid.MustParse("4e19c0ac-94e1-4532-9515-d1801aa283b2")
	TemplateUserAccountDeleted = uuid.MustParse("f44d9314-ad03-4bc8-95d0-5cad491da6b6")

	TemplateUserAccountSuspended = uuid.MustParse("b02ddd82-4733-4d02-a2d7-c36f3598997d")
	TemplateUserAccountActivated = uuid.MustParse("9f5af851-8408-4e73-a7a1-c6502ba46689")
	TemplateYourAccountSuspended = uuid.MustParse("6a2f0609-9b69-4d36-a989-9f5925b6cbff")
	TemplateYourAccountActivated = uuid.MustParse("1a6a6bea-ee0a-43e2-9e7c-eabdb53730e4")

	TemplateUserRequestedOneTimePasscode = uuid.MustParse("62f86a30-2330-4b61-a26d-311ff3b608cf")
)

// Template-related events.
var (
	TemplateTemplateDeleted    = uuid.MustParse("29a09665-2a4c-403f-9648-54301670e7be")
	TemplateTemplateDeprecated = uuid.MustParse("f40fae84-55a2-42cd-99fa-b41c1ca64894")

	TemplateWorkspaceBuildsFailedReport = uuid.MustParse("34a20db2-e9cc-4a93-b0e4-8569699d7a00")
)

// Notification-related events.
var (
	TemplateTestNotification = uuid.MustParse("c425f63e-716a-4bf4-ae24-78348f706c3f")
)
