import type { FC } from "react";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const ChatAccessDeniedAlert: FC = () => {
	const docsLink = docs(
		"/ai-coder/agents/getting-started#step-3-grant-coder-agents-user",
	);

	return (
		<Alert
			severity="info"
			actions={
				<Button size="sm" onClick={() => window.location.reload()}>
					Refresh
				</Button>
			}
		>
			<AlertTitle>Permission required</AlertTitle>
			<AlertDescription>
				You don't have permission to use Coder Agents. Contact your Coder
				administrator for access. Refresh this page after access has been
				granted.{" "}
				<Link href={docsLink} target="_blank" rel="noreferrer">
					View Docs
				</Link>
			</AlertDescription>
		</Alert>
	);
};
