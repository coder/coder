import type { FC } from "react";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const AIBridgeSetupAlert: FC = () => {
	return (
		<Alert className="mb-12" severity="warning" prominent>
			<AlertTitle>
				AI Gateway is included in your license, but not set up yet.
			</AlertTitle>
			<AlertDescription>
				You have access to AI Governance, but it still needs to be set up. Check
				out the{" "}
				<Link href={docs("/ai-coder/ai-gateway")} target="_blank">
					AI Gateway
				</Link>{" "}
				documentation to get started.
			</AlertDescription>
		</Alert>
	);
};
