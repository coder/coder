import type { FC } from "react";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const ChatAccessDeniedAlert: FC = () => {
	const docsLink = docs("/ai-coder/agents/getting-started");

	return (
		<Alert
			severity="info"
			actions={
				<Button size="sm" onClick={() => location.reload()}>
					Refresh
				</Button>
			}
		>
			<AlertTitle>Permission required</AlertTitle>
			<AlertDescription>
				You don't have permission to use Coder Agents. Contact your Coder
				administrator, then refresh this page.{" "}
				<Link href={docsLink} target="_blank">
					View Docs
				</Link>
			</AlertDescription>
		</Alert>
	);
};
