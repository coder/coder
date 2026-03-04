import { API } from "api/api";
import { getErrorMessage } from "api/errors";
import { useState } from "react";
import { useMutation } from "react-query";
import { toast } from "sonner";

export const useDeletionDialogState = (
	templateId: string,
	onDelete: () => void,
	templateName?: string,
) => {
	const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);

	const deleteMutation = useMutation({
		mutationFn: () => API.deleteTemplate(templateId),
	});

	const openDeleteConfirmation = () => {
		setIsDeleteDialogOpen(true);
	};

	const cancelDeleteConfirmation = () => {
		setIsDeleteDialogOpen(false);
	};

	const confirmDelete = () => {
		const label = templateName ? ` "${templateName}"` : "";
		const mutation = deleteMutation.mutateAsync();
		toast.promise(mutation, {
			loading: `Deleting template${label}...`,
			success: `Template${label} deleted successfully.`,
			error: (error) =>
				getErrorMessage(error, `Failed to delete template${label}.`),
		});
		mutation.then(() => onDelete());
	};

	return {
		isDeleteDialogOpen,
		openDeleteConfirmation,
		cancelDeleteConfirmation,
		confirmDelete,
	};
};
