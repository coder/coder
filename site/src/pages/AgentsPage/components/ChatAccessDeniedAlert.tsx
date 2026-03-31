import type { FC } from "react";
import { Alert } from "#/components/Alert/Alert";
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
			className="py-2"
			actions={
				<div className="flex gap-2">
					<Button
						variant="subtle"
						size="sm"
						onClick={() => window.location.reload()}
					>
						Refresh
					</Button>
					<Link href={docsLink} target="_blank" rel="noreferrer" size="sm">
						View Docs
					</Link>
				</div>
			}
		>
			<p className="m-0 font-medium">Permission required</p>
			<p className="m-0 mt-1 text-sm text-content-secondary">
				You don't have permission to create chats. Contact your Coder
				administrator for access. Refresh this page after access has been
				granted.
			</p>
		</Alert>
	);
};
