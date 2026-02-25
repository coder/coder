import { getErrorDetail, getErrorMessage } from "api/errors";
import type { APIKeyWithOwner } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import type { FC } from "react";
import { toast } from "sonner";
import { useDeleteToken } from "./hooks";

interface ConfirmDeleteDialogProps {
	queryKey: (string | boolean)[];
	token: APIKeyWithOwner | undefined;
	setToken: (arg: APIKeyWithOwner | undefined) => void;
}

export const ConfirmDeleteDialog: FC<ConfirmDeleteDialogProps> = ({
	queryKey,
	token,
	setToken,
}) => {
	const tokenName = token?.token_name;

	const { mutate: deleteToken, isPending: isDeleting } =
		useDeleteToken(queryKey);

	const onDeleteSuccess = () => {
		toast.success("Token has been deleted.");
		setToken(undefined);
	};

	const onDeleteError = (error: Error) => {
		const message = getErrorMessage(error, "Failed to delete token");
		toast.error(message, {
			description: getErrorDetail(error),
		});
		setToken(undefined);
	};

	return (
		<ConfirmDialog
			type="delete"
			title="Delete Token"
			description={
				<>
					Are you sure you want to permanently delete token{" "}
					<strong>{tokenName}</strong>?
				</>
			}
			open={Boolean(token) || isDeleting}
			confirmLoading={isDeleting}
			onConfirm={() => {
				if (!token) {
					return;
				}
				deleteToken(token.id, {
					onError: onDeleteError,
					onSuccess: onDeleteSuccess,
				});
			}}
			onClose={() => {
				setToken(undefined);
			}}
		/>
	);
};
