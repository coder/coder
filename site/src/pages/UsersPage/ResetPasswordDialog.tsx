import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";

interface ResetPasswordDialogProps {
	open: boolean;
	onClose: () => void;
	onConfirm: () => void;
	user?: TypesGen.User;
	newPassword?: string;
	loading: boolean;
}

export const ResetPasswordDialog: FC<ResetPasswordDialogProps> = ({
	open,
	onClose,
	onConfirm,
	user,
	newPassword,
	loading,
}) => {
	const description = (
		<>
			<p>
				You will need to send <strong>{user?.username}</strong> the following
				password:
			</p>
			<CodeExample
				secret={false}
				code={newPassword ?? ""}
				className="min-h-auto select-all w-full mt-6"
			/>
		</>
	);

	return (
		<ConfirmDialog
			type="info"
			hideCancel={false}
			open={open}
			onConfirm={onConfirm}
			onClose={onClose}
			title="Reset password"
			confirmLoading={loading}
			confirmText="Reset password"
			description={description}
		/>
	);
};
