import { useMemo } from "react";
import type { ClibaseGroup, ClibaseOption } from "api/typesGenerated";

const deploymentOptions = (
  options: ClibaseOption[],
  ...names: string[]
): ClibaseOption[] => {
  const found: ClibaseOption[] = [];
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
  options: ClibaseOption[],
  ...names: string[]
): ClibaseOption[] => {
  return useMemo(() => deploymentOptions(options, ...names), [options, names]);
};

export const deploymentGroupHasParent = (
  group: ClibaseGroup | undefined,
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
