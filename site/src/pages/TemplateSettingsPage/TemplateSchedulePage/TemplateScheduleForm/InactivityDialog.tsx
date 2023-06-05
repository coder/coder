import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { useWorkspacesData } from "./useWorkspacesData"
import { TemplateScheduleFormValues } from "./formHelpers"

export const InactivityDialog = ({
  formValues,
  submitValues,
  isInactivityDialogOpen,
  setIsInactivityDialogOpen,
}: {
  formValues: TemplateScheduleFormValues
  submitValues: (arg0: TemplateScheduleFormValues) => void
  isInactivityDialogOpen: boolean
  setIsInactivityDialogOpen: (arg0: boolean) => void
}) => {
  const workspacesToBeDeletedToday = useWorkspacesData(formValues)

  return (
    <ConfirmDialog
      type="delete"
      open={isInactivityDialogOpen}
      onConfirm={() => {
        submitValues(formValues)
        setIsInactivityDialogOpen(false)
      }}
      onClose={() => setIsInactivityDialogOpen(false)}
      title="Delete inactive workspaces"
      confirmText="Delete Workspaces"
      description={`There are ${
        workspacesToBeDeletedToday?.length ?? ""
      } workspaces that already match this filter and will be deleted upon form submission. Are you sure you want to proceed?`}
    />
  )
}
