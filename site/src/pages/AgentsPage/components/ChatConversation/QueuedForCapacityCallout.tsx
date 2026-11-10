import { Link as RouterLink } from "react-router";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { docs } from "#/utils/docs";

const concurrencyDocsUrl = docs(
	"/ai-coder/agents/platform-controls#concurrent-agents",
);

export const QueuedForCapacityCallout: React.FC = () => {
	const { entitlements } = useDashboard();
	const { permissions } = useAuthenticated();
	const hasLicense = entitlements.has_license;
	const canManageLicenses = permissions.viewAllLicenses;
	const runtimeHours = entitlements.features.agent_runtime_hours;
	const agentHoursHardLimit =
		runtimeHours.enabled &&
		runtimeHours.hard_limit !== undefined &&
		runtimeHours.actual !== undefined &&
		runtimeHours.actual >= runtimeHours.hard_limit
			? runtimeHours.hard_limit
			: undefined;

	let limitMessage =
		"Your team has reached the Community license limit for active agents.";
	if (hasLicense) {
		limitMessage =
			"Your team has reached your license’s limit for active agents.";
	}
	if (agentHoursHardLimit !== undefined) {
		limitMessage = `Your team has reached the ${agentHoursHardLimit}-hour Agent Hours hard limit.`;
	}

	let action: React.ReactNode = (
		<>
			<Link href={concurrencyDocsUrl} target="_blank" rel="noreferrer">
				Learn more
			</Link>
			.
		</>
	);
	if (canManageLicenses && hasLicense) {
		action = (
			<>
				Contact your Coder account team or{" "}
				<Link href="mailto:sales@coder.com" showExternalIcon={false}>
					sales@coder.com
				</Link>{" "}
				to upgrade to unlimited concurrent agents.
			</>
		);
	} else if (canManageLicenses) {
		action = (
			<>
				<Link asChild showExternalIcon={false}>
					<RouterLink to="/deployment/premium">
						Start an unlimited trial
					</RouterLink>
				</Link>{" "}
				or{" "}
				<Link href={concurrencyDocsUrl} target="_blank" rel="noreferrer">
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
