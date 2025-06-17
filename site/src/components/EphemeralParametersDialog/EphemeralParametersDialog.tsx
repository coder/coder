import type { TemplateVersionParameter } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Link } from "components/Link/Link";
import type { FC } from "react";

interface EphemeralParametersDialogProps {
	open: boolean;
	onClose: () => void;
	onContinue: () => void;
	ephemeralParameters: TemplateVersionParameter[];
	workspaceOwner: string;
	workspaceName: string;
}

export const EphemeralParametersDialog: FC<EphemeralParametersDialogProps> = ({
	open,
	onClose,
	onContinue,
	ephemeralParameters,
	workspaceOwner,
	workspaceName,
}) => {
	const parametersPageUrl = `/@${workspaceOwner}/${workspaceName}/settings/parameters`;

	const description = (
		<>
			<p>This workspace template has ephemeral parameters that will be reset to their default values:</p>
			<div style={{ margin: "16px 0" }}>
				{ephemeralParameters.map((param) => (
					<div key={param.name} style={{ marginBottom: "8px" }}>
						<strong>{param.display_name || param.name}</strong>
						{param.description && (
							<div style={{ fontSize: "14px", color: "#666" }}>
								{param.description}
							</div>
						)}
					</div>
				))}
			</div>
			<p>You can continue without setting values for these parameters, or go to the workspace parameters page to configure them.</p>
			<div style={{ marginTop: "16px" }}>
				<Button asChild onClick={onClose}>
					<Link to={parametersPageUrl}>Go to Parameters Page</Link>
				</Button>
			</div>
		</>
	);

	return (
		<ConfirmDialog
			open={open}
			onClose={onClose}
			onConfirm={onContinue}
			title="Ephemeral Parameters Detected"
			confirmText="Continue Without Setting"
			description={description}
			type="info"
		/>
	);
};