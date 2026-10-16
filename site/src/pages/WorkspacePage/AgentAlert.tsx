import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";

type AgentAlertProps = {
	title: string;
	detail: React.ReactNode;
	severity: "info" | "warning";
	prominent: boolean;
	troubleshootingURL?: string;
};

export const AgentAlert: React.FC<AgentAlertProps> = ({
	title,
	detail,
	severity,
	prominent,
	troubleshootingURL,
}) => {
	return (
		<Alert
			severity={severity}
			prominent={prominent}
			actions={
				troubleshootingURL && (
					<Button size="sm" asChild>
						<a href={troubleshootingURL} target="_blank" rel="noreferrer">
							Go to troubleshooting docs
							<span className="sr-only"> (opens in new tab)</span>
						</a>
					</Button>
				)
			}
		>
			<AlertTitle>{title}</AlertTitle>
			<AlertDescription>{detail}</AlertDescription>
		</Alert>
	);
};
