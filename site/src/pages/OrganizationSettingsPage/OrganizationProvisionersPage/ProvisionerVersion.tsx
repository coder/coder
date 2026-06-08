import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import semver from "semver";
import { StatusIndicator } from "#/components/StatusIndicator/StatusIndicator";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type ProvisionerVersionProps = {
	buildVersion: string | undefined;
	buildAPIVersion: string | undefined;
	provisionerVersion: string;
	provisionerAPIVersion: string;
};

// ProvisionerCompatibility classifies a provisioner daemon against the Coder
// server. Compatibility is governed by the provisioner API version (see
// apiversion.Validate on the server), not the Coder release version.
type ProvisionerCompatibility =
	| "match"
	| "compatible"
	| "outdated"
	| "server-ahead"
	| "unknown";

// getProvisionerCompatibility mirrors apiversion.Validate at a coarse level:
// a daemon is compatible when its API version has the same major as the
// server and a minor that is less than or equal. Servers may accept older
// majors via backward-compat lists the UI does not see, so older majors are
// reported as "outdated" rather than incompatible.
export const getProvisionerCompatibility = (
	buildVersion: string | undefined,
	buildAPIVersion: string | undefined,
	provisionerVersion: string,
	provisionerAPIVersion: string,
): ProvisionerCompatibility => {
	if (buildVersion && provisionerVersion === buildVersion) {
		return "match";
	}

	const prov = semver.coerce(provisionerAPIVersion);
	const srv = semver.coerce(buildAPIVersion ?? "");
	if (!prov || !srv) {
		return "unknown";
	}

	if (prov.major === srv.major) {
		return prov.minor <= srv.minor ? "compatible" : "server-ahead";
	}
	if (prov.major > srv.major) {
		return "server-ahead";
	}
	return "outdated";
};

export const provisionerCompatibilityLabel: Record<
	ProvisionerCompatibility,
	string
> = {
	match: "Up to date",
	compatible: "Compatible",
	outdated: "Outdated",
	"server-ahead": "Server out of date",
	unknown: "Version mismatch",
};

export const ProvisionerVersion: FC<ProvisionerVersionProps> = ({
	buildVersion,
	buildAPIVersion,
	provisionerVersion,
	provisionerAPIVersion,
}) => {
	const status = getProvisionerCompatibility(
		buildVersion,
		buildAPIVersion,
		provisionerVersion,
		provisionerAPIVersion,
	);
	const label = provisionerCompatibilityLabel[status];

	if (status === "match") {
		return (
			<span className="text-xs font-medium text-content-secondary">
				{label}
			</span>
		);
	}

	if (status === "compatible") {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<StatusIndicator
						variant="inactive"
						size="sm"
						className="cursor-pointer"
						tabIndex={0}
					>
						{label}
					</StatusIndicator>
				</TooltipTrigger>
				<TooltipContent className="max-w-xs">
					<p className="m-0">
						This provisioner ({provisionerVersion}) and the Coder server (
						{buildVersion}) report different release versions, but share a
						compatible provisioner API ({provisionerAPIVersion} vs{" "}
						{buildAPIVersion}). No action is required.
					</p>
				</TooltipContent>
			</Tooltip>
		);
	}

	if (status === "server-ahead") {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<StatusIndicator
						variant="warning"
						size="sm"
						className="cursor-pointer"
						tabIndex={0}
					>
						<TriangleAlertIcon className="size-icon-xs" />
						{label}
					</StatusIndicator>
				</TooltipTrigger>
				<TooltipContent className="max-w-xs">
					<p className="m-0">
						This provisioner reports API version {provisionerAPIVersion}, which
						is newer than the Coder server (API {buildAPIVersion}). Upgrade the
						server, or downgrade this provisioner to a compatible version.
					</p>
				</TooltipContent>
			</Tooltip>
		);
	}

	// "outdated" or "unknown".
	const description =
		status === "outdated"
			? `This provisioner reports API version ${provisionerAPIVersion}, which is older than the Coder server (API ${buildAPIVersion}). Upgrading the provisioner is recommended.`
			: `This provisioner is on ${provisionerVersion}, while the Coder server is on ${buildVersion ?? "an unknown version"}. The provisioner API version could not be compared, so compatibility cannot be verified.`;

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<StatusIndicator
					variant="warning"
					size="sm"
					className="cursor-pointer"
					tabIndex={0}
				>
					<TriangleAlertIcon className="size-icon-xs" />
					{label}
				</StatusIndicator>
			</TooltipTrigger>
			<TooltipContent className="max-w-xs">
				<p className="m-0">{description}</p>
			</TooltipContent>
		</Tooltip>
	);
};
