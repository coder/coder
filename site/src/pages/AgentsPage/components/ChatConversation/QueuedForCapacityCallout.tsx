import type { FC } from "react";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";

interface QueuedForCapacityCalloutProps {
	hasLicense: boolean;
	canManageLicenses: boolean;
}

export const QueuedForCapacityCallout: FC<QueuedForCapacityCalloutProps> = ({
	hasLicense,
	canManageLicenses,
}) => {
	return (
		<Alert severity="warning" className="mt-2">
			<AlertDescription>
				{hasLicense
					? "Your team has reached your license’s limit for active agents."
					: "Your team has reached the Community license limit for active agents."}{" "}
				This agent is queued and will start automatically when capacity is
				available.{" "}
				{canManageLicenses ? (
					hasLicense ? (
						<>
							Contact your Coder account team or{" "}
							<Link
								href="mailto:sales@coder.com"
								size="sm"
								showExternalIcon={false}
							>
								sales@coder.com
							</Link>{" "}
							to upgrade to unlimited concurrent agents.
						</>
					) : (
						<>
							<Link
								href="https://coder.com/trial"
								target="_blank"
								rel="noreferrer"
								size="sm"
							>
								Start an unlimited trial
							</Link>{" "}
							or{" "}
							<Link
								href="https://coder.com/pricing"
								target="_blank"
								rel="noreferrer"
								size="sm"
							>
								learn more
							</Link>
							.
						</>
					)
				) : (
					<>
						<Link
							href="https://coder.com/pricing"
							target="_blank"
							rel="noreferrer"
							size="sm"
						>
							Learn more
						</Link>
						.
					</>
				)}
			</AlertDescription>
		</Alert>
	);
};
