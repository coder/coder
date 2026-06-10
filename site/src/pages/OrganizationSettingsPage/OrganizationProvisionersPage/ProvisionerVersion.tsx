import { TriangleAlertIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import semver from "semver";
import {
	StatusIndicator,
	type StatusIndicatorProps,
} from "#/components/StatusIndicator/StatusIndicator";
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

// getProvisionerCompatibility mirrors apiversion.Validate: a daemon is
// compatible when its API version has the same major as the server and a
// minor that is less than or equal. The server registers no backward-compat
// majors, so it rejects a daemon with an older API major at connect time,
// and that daemon cannot reconnect until it is upgraded.
export const getProvisionerCompatibility = (
	buildVersion: string | undefined,
	buildAPIVersion: string | undefined,
	provisionerVersion: string,
	provisionerAPIVersion: string,
): ProvisionerCompatibility => {
	// Build info loads asynchronously. Until both the server release version
	// and its provisioner API version are known, compatibility cannot be
	// assessed, so report "unknown" instead of a warning.
	if (!buildVersion || !buildAPIVersion) {
		return "unknown";
	}
	if (provisionerVersion === buildVersion) {
		return "match";
	}

	const prov = semver.coerce(provisionerAPIVersion);
	const srv = semver.coerce(buildAPIVersion);
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
	unknown: "Unknown",
};

type StatusConfig = {
	variant: StatusIndicatorProps["variant"];
	icon?: ReactNode;
	description: string;
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

	const statusConfig: Record<
		Exclude<ProvisionerCompatibility, "match">,
		StatusConfig
	> = {
		compatible: {
			variant: "inactive",
			description: `This provisioner (${provisionerVersion}) and the Coder server (${buildVersion}) report different release versions, but share a compatible provisioner API (${provisionerAPIVersion} vs ${buildAPIVersion}). No action is required.`,
		},
		"server-ahead": {
			variant: "warning",
			icon: <TriangleAlertIcon className="size-icon-xs" />,
			description: `This provisioner reports API version ${provisionerAPIVersion}, which is newer than the Coder server (API ${buildAPIVersion}). Upgrade the server, or downgrade this provisioner to a compatible version.`,
		},
		outdated: {
			variant: "warning",
			icon: <TriangleAlertIcon className="size-icon-xs" />,
			description: `This provisioner reports API version ${provisionerAPIVersion}, which is incompatible with the Coder server (API ${buildAPIVersion}). The server rejects incompatible provisioners, so this provisioner cannot reconnect until it is upgraded.`,
		},
		unknown: {
			variant: "inactive",
			description: `This provisioner is on ${provisionerVersion}, while the Coder server is on ${buildVersion ?? "an unknown version"}. The provisioner API version could not be compared, so compatibility cannot be verified.`,
		},
	};
	const config = statusConfig[status];

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<StatusIndicator
					variant={config.variant}
					size="sm"
					className="cursor-pointer"
					role="status"
					tabIndex={0}
				>
					{config.icon}
					{label}
				</StatusIndicator>
			</TooltipTrigger>
			<TooltipContent className="max-w-xs">
				<p className="m-0">{config.description}</p>
			</TooltipContent>
		</Tooltip>
	);
};
