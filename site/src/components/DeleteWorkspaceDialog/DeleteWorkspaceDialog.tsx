import React from "react"
import { ConfirmDialog } from "../ConfirmDialog/ConfirmDialog"

const Language = {
  deleteDialogTitle: "Delete workspace?",
  deleteDialogMessage: "Deleting your workspace is irreversible. Are you sure?",
}

export interface DeleteWorkspaceDialogProps {
  isOpen: boolean
  handleConfirm: () => void
  handleCancel: () => void
}

export const DeleteWorkspaceDialog: React.FC<
  React.PropsWithChildren<DeleteWorkspaceDialogProps>
> = ({ isOpen, handleCancel, handleConfirm }) => (
  <ConfirmDialog
    type="delete"
    hideCancel={false}
    open={isOpen}
    title={Language.deleteDialogTitle}
    onConfirm={handleConfirm}
    onClose={handleCancel}
    description={Language.deleteDialogMessage}
  />
)
