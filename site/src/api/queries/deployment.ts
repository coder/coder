import * as API from "api/api";

export const deploymentConfig = () => {
  return {
    queryKey: ["deployment", "config"],
    queryFn: API.getDeploymentConfig,
  };
};

export const deploymentDAUs = () => {
  return {
    queryKey: ["deployment", "daus"],
    queryFn: () => API.getDeploymentDAUs(),
  };
};
