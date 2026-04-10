import type { FC } from "react";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";

interface WorkspaceAlertProps {
	title: string;
	detail: string;
	severity: "info" | "warning";
	prominent: boolean;
	troubleshootingURL: string | undefined;
}

export const WorkspaceAlert: FC<WorkspaceAlertProps> = ({
	title,
	detail,
	severity,
	prominent,
	troubleshootingURL,
}) => {
	return (
		<Alert severity={severity} prominent={prominent}>
			<AlertTitle>{title}</AlertTitle>
			<AlertDescription>
				<div className="mb-2">{detail}</div>
				{troubleshootingURL && (
					<Button asChild>
						<a href={troubleshootingURL} target="_blank" rel="noopener">
							View docs to troubleshoot
						</a>
					</Button>
				)}
			</AlertDescription>
		</Alert>
	);
};
