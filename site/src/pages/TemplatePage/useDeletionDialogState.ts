import { useState } from "react";
import { API } from "api/api";
import { getErrorMessage } from "api/errors";
import { displayError } from "components/GlobalSnackbar/utils";

type DeleteTemplateState =
  | { status: "idle" }
  | { status: "confirming" }
  | { status: "deleting" };

export const useDeletionDialogState = (
  templateId: string,
  onDelete: () => void,
) => {
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
      await API.deleteTemplate(templateId);
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
