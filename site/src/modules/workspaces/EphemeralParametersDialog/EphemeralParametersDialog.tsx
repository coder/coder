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
import { useNavigate } from "react-router";

interface EphemeralParametersDialogProps {
	open: boolean;
	onClose: () => void;
	onContinue: () => void;
	ephemeralParameters: TemplateVersionParameter[];
	workspaceOwner: string;
	workspaceName: string;
	templateVersionId: string;
}

export const EphemeralParametersDialog: FC<EphemeralParametersDialogProps> = ({
	open,
	onClose,
	onContinue,
	ephemeralParameters,
	workspaceOwner,
	workspaceName,
	templateVersionId,
}) => {
	const navigate = useNavigate();

	const handleGoToParameters = () => {
		onClose();
		navigate(
			`/@${workspaceOwner}/${workspaceName}/settings/parameters?templateVersionId=${templateVersionId}`,
		);
	};

	return (
		<Dialog open={open} onOpenChange={(isOpen) => !isOpen && onClose()}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Ephemeral parameters detected</DialogTitle>
					<DialogDescription>
						This workspace template has{" "}
						<strong className="text-content-primary">
							{ephemeralParameters.length}
						</strong>{" "}
						ephemeral parameters that will be reset to their default values
					</DialogDescription>
					<DialogDescription>
						<ul className="list-none pl-6 space-y-2">
							{ephemeralParameters.map((param) => (
								<li key={param.name}>
									<p className="text-content-primary m-0 font-bold">
										{param.display_name || param.name}
									</p>
									{param.description && (
										<p className="m-0 text-sm text-content-secondary">
											{param.description}
										</p>
									)}
								</li>
							))}
						</ul>
					</DialogDescription>
					<DialogDescription>
						Would you like to go to the workspace parameters page to review and
						update these parameters before continuing?
					</DialogDescription>
				</DialogHeader>
				<DialogFooter>
					<Button onClick={onContinue} variant="outline">
						Continue
					</Button>
					<Button
						data-testid="workspace-parameters"
						onClick={handleGoToParameters}
					>
						Go to workspace parameters
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
