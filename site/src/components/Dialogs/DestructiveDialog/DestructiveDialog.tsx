import React, { ReactNode } from "react"
import { ConfirmDialog } from "../ConfirmDialog/ConfirmDialog"

export interface DestructiveDialogProps {
  isOpen: boolean
  handleConfirm: () => void
  handleCancel: () => void
  title: string
  description: string | ReactNode
  confirmLoading?: boolean
}

export const DestructiveDialog: React.FC<React.PropsWithChildren<DestructiveDialogProps>> = ({
  isOpen,
  handleCancel,
  handleConfirm,
  title,
  description,
  confirmLoading,
}) => (
  <ConfirmDialog
    type="delete"
    hideCancel={false}
    open={isOpen}
    title={title}
    onConfirm={handleConfirm}
    onClose={handleCancel}
    description={description}
    confirmLoading={confirmLoading}
  />
)
