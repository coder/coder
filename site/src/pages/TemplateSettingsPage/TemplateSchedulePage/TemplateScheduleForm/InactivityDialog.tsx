import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"

export const InactivityDialog = ({
  submitValues,
  isInactivityDialogOpen,
  setIsInactivityDialogOpen,
  workspacesToBeDeletedToday,
}: {
  submitValues: () => void
  isInactivityDialogOpen: boolean
  setIsInactivityDialogOpen: (arg0: boolean) => void
  workspacesToBeDeletedToday: number
}) => {
  return (
    <ConfirmDialog
      type="delete"
      open={isInactivityDialogOpen}
      onConfirm={() => {
        submitValues()
        setIsInactivityDialogOpen(false)
      }}
      onClose={() => setIsInactivityDialogOpen(false)}
      title="Delete inactive workspaces"
      confirmText="Delete Workspaces"
      description={`There are ${
        workspacesToBeDeletedToday ? workspacesToBeDeletedToday : ""
      } workspaces that already match this filter and will be deleted upon form submission. Are you sure you want to proceed?`}
    />
  )
}
