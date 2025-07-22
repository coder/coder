import type { FieldError } from "api/errors";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import type { FC } from "react";
import { useNavigate } from "react-router";

type UpdateBuildParametersDialogExperimentalProps = {
	open: boolean;
	onClose: () => void;
	validations: FieldError[];
	workspaceOwnerName: string;
	workspaceName: string;
	templateVersionId: string | undefined;
};

export const UpdateBuildParametersDialogExperimental: FC<
	UpdateBuildParametersDialogExperimentalProps
> = ({
	validations,
	open,
	onClose,
	workspaceOwnerName,
	workspaceName,
	templateVersionId,
}) => {
	const navigate = useNavigate();

	const handleGoToParameters = () => {
		onClose();
		navigate(
			`/@${workspaceOwnerName}/${workspaceName}/settings/parameters?templateVersionId=${templateVersionId}`,
		);
	};

	return (
		<Dialog open={open} onOpenChange={(isOpen) => !isOpen && onClose()}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Update workspace parameters</DialogTitle>
					<DialogDescription>
						This template has{" "}
						<strong className="text-content-primary">
							{validations.length} parameter
							{validations.length === 1 ? "" : "s"}
						</strong>{" "}
						that must be configured to complete the update.
					</DialogDescription>
					<DialogDescription>
						Would you like to go to the workspace parameters page to review and
						update these parameters before continuing?
					</DialogDescription>
				</DialogHeader>
				<DialogFooter>
					<Button onClick={onClose} variant="outline">
						Cancel
					</Button>
					<Button onClick={handleGoToParameters}>
						Go to workspace parameters
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
