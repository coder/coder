import type { FC } from "react";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { docs } from "#/utils/docs";

const docsLink = docs(
	"/ai-coder/agents/getting-started#step-3-grant-use-coder-agents",
);

export const ChatAccessDeniedAlert: FC = () => (
	<Alert
		severity="info"
		className="py-2"
		actions={
			<Button asChild variant="subtle" size="sm">
				<a href={docsLink} target="_blank" rel="noreferrer">
					View Docs
				</a>
			</Button>
		}
	>
		<p className="m-0 font-medium">Use Coder Agents role required</p>
		<p className="m-0 mt-1 text-sm text-content-secondary">
			You don't have permission to create chats. Ask your Coder administrator to
			assign you the "<strong>Use Coder Agents</strong>" role from{" "}
			<strong>Admin &gt; Users</strong>.
		</p>
	</Alert>
);
