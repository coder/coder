import type { FC, ReactNode } from "react";
import type { AgentScriptTiming } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";

interface AgentAlertProps {
	title: string;
	detail: ReactNode;
	severity: "info" | "warning";
	prominent: boolean;
	troubleshootingURL?: string;
}

interface StartScriptFailureDetailProps {
	baseDetail: string;
	timings: readonly AgentScriptTiming[];
}

export const StartScriptFailureDetail: FC<StartScriptFailureDetailProps> = ({
	baseDetail,
	timings,
}) => {
	return (
		<>
			{baseDetail}
			<ul className="mt-2 mb-0 pl-0 list-none space-y-0.5">
				{timings.map((t) => (
					<li key={t.display_name} className="font-mono text-xs">
						&ldquo;{t.display_name}&rdquo; exited with code {t.exit_code}
					</li>
				))}
			</ul>
		</>
	);
};

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
