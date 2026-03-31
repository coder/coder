import type { FC } from "react";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const AIBridgeSetupAlert: FC = () => {
	return (
		<Alert className="mb-12" severity="warning" prominent>
			<AlertTitle>
				AI Bridge is included in your license, but not set up yet.
			</AlertTitle>
			<AlertDescription>
				You have access to AI Governance, but it still needs to be setup. Check
				out the{" "}
				<Link href={docs("/ai-coder/ai-bridge")} target="_blank">
					AI Bridge
				</Link>{" "}
				documentation to get started.
			</AlertDescription>
		</Alert>
	);
};
