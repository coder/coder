import type { FC, ReactNode } from "react";
import { Link as RouterLink } from "react-router";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

const concurrencyDocsUrl = docs(
	"/ai-coder/agents/platform-controls#concurrent-agents",
);

interface QueuedForCapacityCalloutProps {
	hasLicense: boolean;
	canManageLicenses: boolean;
	agentHoursHardLimit?: number;
}

export const QueuedForCapacityCallout: FC<QueuedForCapacityCalloutProps> = ({
	hasLicense,
	canManageLicenses,
	agentHoursHardLimit,
}) => {
	let limitMessage =
		"Your team has reached the Community license limit for active agents.";
	if (hasLicense) {
		limitMessage =
			"Your team has reached your license’s limit for active agents.";
	}
	if (agentHoursHardLimit !== undefined) {
		limitMessage = `Your team has reached the ${agentHoursHardLimit}-hour Agent Hours hard limit.`;
	}

	let action: ReactNode = (
		<>
			<Link
				href={concurrencyDocsUrl}
				target="_blank"
				rel="noreferrer"
				size="sm"
			>
				Learn more
			</Link>
			.
		</>
	);
	if (canManageLicenses && hasLicense) {
		action = (
			<>
				Contact your Coder account team or{" "}
				<Link href="mailto:sales@coder.com" size="sm" showExternalIcon={false}>
					sales@coder.com
				</Link>{" "}
				to upgrade to unlimited concurrent agents.
			</>
		);
	} else if (canManageLicenses) {
		action = (
			<>
				<Link asChild showExternalIcon={false} size="sm">
					<RouterLink to="/deployment/premium">
						Start an unlimited trial
					</RouterLink>
				</Link>{" "}
				or{" "}
				<Link
					href={concurrencyDocsUrl}
					target="_blank"
					rel="noreferrer"
					size="sm"
				>
					learn more
				</Link>
				.
			</>
		);
	}

	return (
		<Alert severity="warning" className="mt-2">
			<AlertDescription>
				{limitMessage} This agent is queued and will start automatically when
				capacity is available. {action}
			</AlertDescription>
		</Alert>
	);
};
