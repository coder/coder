import { getErrorDetail, getErrorMessage, isApiError } from "api/errors";
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

interface WorkspaceErrorDialogProps {
	open: boolean;
	error?: unknown;
	onClose: () => void;
	showDetail: boolean;
	workspaceOwner: string;
	workspaceName: string;
	templateVersionId: string;
	isDeleting: boolean;
}

export const WorkspaceErrorDialog: FC<WorkspaceErrorDialogProps> = ({
	open,
	error,
	onClose,
	showDetail,
	workspaceOwner,
	workspaceName,
	templateVersionId,
	isDeleting,
}) => {
	const navigate = useNavigate();

	if (!error) {
		return null;
	}

	const handleGoToParameters = () => {
		onClose();
		navigate(
			`/@${workspaceOwner}/${workspaceName}/settings/parameters?templateVersionId=${templateVersionId}`,
		);
	};

	const errorDetail = getErrorDetail(error);
	const validations = isApiError(error)
		? error.response.data.validations
		: undefined;

	return (
		<Dialog open={open} onOpenChange={(isOpen) => !isOpen && onClose()}>
			<DialogContent variant="destructive">
				<DialogHeader>
					<DialogTitle>
						Error {isDeleting ? "deleting" : "building"} workspace
					</DialogTitle>
					<DialogDescription className="flex flex-row gap-4">
						<strong className="text-content-primary">Message</strong>{" "}
						<span>{getErrorMessage(error, "Failed to build workspace.")}</span>
					</DialogDescription>
					{errorDetail && showDetail && (
						<DialogDescription className="flex flex-row gap-9">
							<strong className="text-content-primary">Detail</strong>{" "}
							<span>{errorDetail}</span>
						</DialogDescription>
					)}
					{validations && (
						<DialogDescription className="flex flex-row gap-4">
							<strong className="text-content-primary">Validations</strong>{" "}
							<span>
								{validations.map((validation) => validation.detail).join(", ")}
							</span>
						</DialogDescription>
					)}
				</DialogHeader>
				<DialogFooter>
					<Button onClick={handleGoToParameters}>
						Review workspace settings
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
