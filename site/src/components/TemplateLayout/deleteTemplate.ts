import { deleteTemplate } from "api/api"
import { Template } from "api/typesGenerated"
import { useState } from "react"

type DeleteTemplateState =
  | { status: "idle" }
  | { status: "confirming" }
  | { status: "deleting" }

export const useDeleteTemplate = (template: Template, onDelete: () => void) => {
  const [state, setState] = useState<DeleteTemplateState>({ status: "idle" })
  const isDeleteDialogOpen =
    state.status === "confirming" || state.status === "deleting"

  const openDeleteConfirmation = () => {
    setState({ status: "confirming" })
  }

  const cancelDeleteConfirmation = () => {
    setState({ status: "idle" })
  }

  const confirmDelete = async () => {
    setState({ status: "deleting" })
    await deleteTemplate(template.id)
    onDelete()
  }

  return {
    state,
    isDeleteDialogOpen,
    openDeleteConfirmation,
    cancelDeleteConfirmation,
    confirmDelete,
  }
}
