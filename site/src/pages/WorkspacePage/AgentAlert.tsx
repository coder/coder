import type { FC } from "react";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";

interface AgentAlertProps {
	title: string;
	detail: string;
	severity: "info" | "warning";
	prominent: boolean;
	troubleshootingURL?: string;
}

export const AgentAlert: FC<AgentAlertProps> = ({
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
					<Button size="sm" asChild>
						<a href={troubleshootingURL} target="_blank" rel="noopener">
							View docs to troubleshoot
						</a>
					</Button>
				)}
			</AlertDescription>
		</Alert>
	);
};
