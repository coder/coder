import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"

export const InactivityDialog = ({
  submitValues,
  isInactivityDialogOpen,
  setIsInactivityDialogOpen,
  workspacesToBeLockedToday,
}: {
  submitValues: () => void
  isInactivityDialogOpen: boolean
  setIsInactivityDialogOpen: (arg0: boolean) => void
  workspacesToBeLockedToday: number
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
      title="Lock inactive workspaces"
      confirmText="Lock Workspaces"
      description={`There are ${
        workspacesToBeLockedToday ? workspacesToBeLockedToday : ""
      } workspaces that already match this filter and will be locked upon form submission. Are you sure you want to proceed?`}
    />
  )
}

export const DeleteLockedDialog = ({
  submitValues,
  isLockedDialogOpen,
  setIsLockedDialogOpen,
  workspacesToBeDeletedToday,
}: {
  submitValues: () => void
  isLockedDialogOpen: boolean
  setIsLockedDialogOpen: (arg0: boolean) => void
  workspacesToBeDeletedToday: number
}) => {
  return (
    <ConfirmDialog
      type="delete"
      open={isLockedDialogOpen}
      onConfirm={() => {
        submitValues()
        setIsLockedDialogOpen(false)
      }}
      onClose={() => setIsLockedDialogOpen(false)}
      title="Delete Locked Workspaces"
      confirmText="Delete Workspaces"
      description={`There are ${
        workspacesToBeDeletedToday ? workspacesToBeDeletedToday : ""
      } workspaces that already match this filter and will be deleted upon form submission. Are you sure you want to proceed?`}
    />
  )
}
