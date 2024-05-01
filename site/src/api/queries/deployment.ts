import { client } from "api/api";

export const deploymentConfig = () => {
  return {
    queryKey: ["deployment", "config"],
    queryFn: client.api.getDeploymentConfig,
  };
};

export const deploymentDAUs = () => {
  return {
    queryKey: ["deployment", "daus"],
    queryFn: () => client.api.getDeploymentDAUs(),
  };
};

export const deploymentStats = () => {
  return {
    queryKey: ["deployment", "stats"],
    queryFn: client.api.getDeploymentStats,
  };
};

export const deploymentSSHConfig = () => {
  return {
    queryKey: ["deployment", "sshConfig"],
    queryFn: client.api.getDeploymentSSHConfig,
  };
};
