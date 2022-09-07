import React, { ReactNode } from "react"
import { ConfirmDialog } from "../ConfirmDialog/ConfirmDialog"

export interface DeleteDialogProps {
  isOpen: boolean
  onConfirm: () => void
  onCancel: () => void
  title: string
  description: string | ReactNode
  confirmLoading?: boolean
}

export const DeleteDialog: React.FC<React.PropsWithChildren<DeleteDialogProps>> = ({
  isOpen,
  onCancel,
  onConfirm,
  title,
  description,
  confirmLoading,
}) => (
  <ConfirmDialog
    type="delete"
    hideCancel={false}
    open={isOpen}
    title={title}
    onConfirm={onConfirm}
    onClose={onCancel}
    description={description}
    confirmLoading={confirmLoading}
  />
)
