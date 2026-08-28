import { API } from "#/api/api";
import type { UsagePeriod } from "#/api/typesGenerated";
import { disabledRefetchOptions } from "./util";

export const deploymentConfigQueryKey = ["deployment", "config"];

export const deploymentConfig = () => {
	return {
		queryKey: deploymentConfigQueryKey,
		queryFn: API.getDeploymentConfig,
		staleTime: Number.POSITIVE_INFINITY,
	};
};

export const deploymentDAUs = () => {
	return {
		queryKey: ["deployment", "daus"],
		queryFn: () => API.getDeploymentDAUs(),
	};
};

const deploymentAgentTimeQueryKey = ["deployment", "agentTime"] as const;

export const deploymentAgentTime = (usagePeriod?: UsagePeriod) => {
	return {
		queryKey: usagePeriod
			? [...deploymentAgentTimeQueryKey, usagePeriod.start, usagePeriod.end]
			: deploymentAgentTimeQueryKey,
		queryFn: API.getDeploymentAgentTime,
		staleTime: 5 * 60 * 1_000,
	};
};

export const deploymentStatsQueryKey = ["deployment", "stats"];

export const deploymentStats = () => {
	return {
		queryKey: deploymentStatsQueryKey,
		queryFn: API.getDeploymentStats,
	};
};

export const deploymentSSHConfig = () => {
	return {
		...disabledRefetchOptions,
		queryKey: ["deployment", "sshConfig"],
		queryFn: API.getDeploymentSSHConfig,
	};
};

export const deploymentIdpSyncFieldValues = (field: string) => {
	return {
		queryKey: ["deployment", "idpSync", "fieldValues", field],
		queryFn: () => API.getDeploymentIdpSyncFieldValues(field),
	};
};
