import { API } from "api/api";

export const deploymentConfigQueryKey = ["deployment", "config"];

export const deploymentConfig = () => {
	return {
		queryKey: deploymentConfigQueryKey,
		queryFn: API.getDeploymentConfig,
	};
};

export const deploymentDAUs = () => {
	return {
		queryKey: ["deployment", "daus"],
		queryFn: () => API.getDeploymentDAUs(),
	};
};

export const deploymentStats = () => {
	return {
		queryKey: ["deployment", "stats"],
		queryFn: API.getDeploymentStats,
	};
};

export const deploymentSSHConfig = () => {
	return {
		queryKey: ["deployment", "sshConfig"],
		queryFn: API.getDeploymentSSHConfig,
	};
};
