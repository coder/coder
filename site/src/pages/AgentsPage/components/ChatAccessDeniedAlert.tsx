import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

type ChatAccessDeniedAlertProps = {
	readonly title?: string;
	readonly description?: string;
};

export const ChatAccessDeniedAlert: React.FC<ChatAccessDeniedAlertProps> = ({
	title = "You can't use Coder Agents",
	description = "Your existing chats are kept and come back when access is restored. Ask your Coder administrator for access, then refresh.",
}) => {
	const docsLink = docs(
		"/ai-coder/agents/getting-started#control-who-can-use-coder-agents",
	);

	return (
		<Alert
			severity="warning"
			actions={
				<>
					<Button size="sm" onClick={() => location.reload()}>
						Refresh
					</Button>
					<Link href={docsLink} target="_blank" rel="noreferrer">
						Go to access docs
						<span className="sr-only"> (opens in new tab)</span>
					</Link>
				</>
			}
		>
			<AlertTitle>{title}</AlertTitle>
			<AlertDescription>{description}</AlertDescription>
		</Alert>
	);
};
