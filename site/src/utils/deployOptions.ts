import { useMemo } from "react";
import { DeploymentGroup, DeploymentOption } from "../api/types";

const deploymentOptions = (
  options: DeploymentOption[],
  ...names: string[]
): DeploymentOption[] => {
  const found: DeploymentOption[] = [];
  for (const name of names) {
    const option = options.find((o) => o.name === name);
    if (option) {
      found.push(option);
    } else {
      throw new Error(`Deployment option ${name} not found`);
    }
  }
  return found;
};

export const useDeploymentOptions = (
  options: DeploymentOption[],
  ...names: string[]
): DeploymentOption[] => {
  return useMemo(() => deploymentOptions(options, ...names), [options, names]);
};

export const deploymentGroupHasParent = (
  group: DeploymentGroup | undefined,
  parent: string,
): boolean => {
  if (!group) {
    return false;
  }
  if (group.parent) {
    return deploymentGroupHasParent(group.parent, parent);
  }
  if (group.name === parent) {
    return true;
  }
  return false;
};
