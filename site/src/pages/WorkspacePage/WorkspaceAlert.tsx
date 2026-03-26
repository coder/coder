import type { FC } from "react";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";

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
				<p>{detail}</p>
				<p>
					{troubleshootingURL && (
						<Link href={troubleshootingURL} target="_blank">
							View docs to troubleshoot
						</Link>
					)}
				</p>
			</AlertDescription>
		</Alert>
	);
};
