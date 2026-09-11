import type { FC } from "react";
import { toast } from "sonner";
import type { APIKeyWithOwner } from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
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

	const {
		mutate: deleteToken,
		isPending: isDeleting,
		error,
		reset,
	} = useDeleteToken(queryKey);

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
			open={Boolean(token)}
			confirmLoading={isDeleting}
			error={error}
			errorMessage="Failed to delete token."
			onConfirm={() => {
				if (!token) {
					return;
				}
				deleteToken(token.id, {
					onSuccess: () => {
						toast.success("Token has been deleted.");
						setToken(undefined);
					},
				});
			}}
			onClose={() => {
				reset();
				setToken(undefined);
			}}
		/>
	);
};
