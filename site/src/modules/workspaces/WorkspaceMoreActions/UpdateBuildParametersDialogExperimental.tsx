import type { TemplateVersionParameter } from "api/typesGenerated";
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
import { useNavigate } from "react-router-dom";

type UpdateBuildParametersDialogExperimentalProps = {
	open: boolean;
	onClose: () => void;
	missedParameters: TemplateVersionParameter[];
	workspaceOwnerName: string;
	workspaceName: string;
	templateVersionId: string | undefined;
};

export const UpdateBuildParametersDialogExperimental: FC<
	UpdateBuildParametersDialogExperimentalProps
> = ({
	missedParameters,
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
							{missedParameters.length} ephemeral parameter
							{missedParameters.length === 1 ? "" : "s"}
						</strong>{" "}
						that need to be configured before continuing. Ephemeral parameters may depend on values of non-ephemeral parameters.
					</DialogDescription>
					<DialogDescription>
						Please go to the workspace settings page to provide values for these ephemeral parameters before continuing.
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
