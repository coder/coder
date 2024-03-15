import { useMemo } from "react";
import type { SerpentGroup, SerpentOption } from "api/typesGenerated";

const deploymentOptions = (
  options: SerpentOption[],
  ...names: string[]
): SerpentOption[] => {
  const found: SerpentOption[] = [];
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
  options: SerpentOption[],
  ...names: string[]
): SerpentOption[] => {
  return useMemo(() => deploymentOptions(options, ...names), [options, names]);
};

export const deploymentGroupHasParent = (
  group: SerpentGroup | undefined,
  parent: string,
): boolean => {
  if (!group) {
    return false;
  }
  if (group.name === parent) {
    return true;
  }
  if (group.parent) {
    return deploymentGroupHasParent(group.parent, parent);
  }
  return false;
};
