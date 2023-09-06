import { deleteTemplate } from "api/api";
import { getErrorMessage } from "api/errors";
import { Template } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useState } from "react";

type DeleteTemplateState =
  | { status: "idle" }
  | { status: "confirming" }
  | { status: "deleting" };

export const useDeleteTemplate = (template: Template, onDelete: () => void) => {
  const [state, setState] = useState<DeleteTemplateState>({ status: "idle" });
  const isDeleteDialogOpen =
    state.status === "confirming" || state.status === "deleting";

  const openDeleteConfirmation = () => {
    setState({ status: "confirming" });
  };

  const cancelDeleteConfirmation = () => {
    setState({ status: "idle" });
  };

  const confirmDelete = async () => {
    try {
      setState({ status: "deleting" });
      await deleteTemplate(template.id);
      onDelete();
    } catch (e) {
      setState({ status: "confirming" });
      displayError(getErrorMessage(e, "Failed to delete template"));
    }
  };

  return {
    state,
    isDeleteDialogOpen,
    openDeleteConfirmation,
    cancelDeleteConfirmation,
    confirmDelete,
  };
};
