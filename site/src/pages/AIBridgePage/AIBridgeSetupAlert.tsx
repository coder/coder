import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const AIBridgeSetupAlert: React.FC = () => {
	return (
		<Alert
			className="mb-12"
			severity="info"
			actions={
				<Link
					href={docs("/ai-coder/ai-gateway")}
					target="_blank"
					rel="noreferrer"
				>
					Go to setup guide
					<span className="sr-only"> (opens in new tab)</span>
				</Link>
			}
		>
			<AlertTitle>AI Gateway isn't set up yet</AlertTitle>
			<AlertDescription>
				Your license includes it, but it isn't turned on for this deployment.
				Follow the setup guide to start monitoring and managing AI requests.
			</AlertDescription>
		</Alert>
	);
};
