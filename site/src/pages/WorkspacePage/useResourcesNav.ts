import { WorkspaceResource } from "api/typesGenerated";
import { useTab } from "hooks";
import { useCallback, useEffect } from "react";

export const resourceOptionId = (resource: WorkspaceResource) => {
  return `${resource.type}_${resource.name}`;
};

export const useResourcesNav = (resources: WorkspaceResource[]) => {
  const resourcesNav = useTab("resources", "");
  const selectedResource = resources.find(
    (r) => resourceOptionId(r) === resourcesNav.value,
  );

  useEffect(() => {
    const hasResourcesWithAgents =
      resources.length > 0 &&
      resources[0].agents &&
      resources[0].agents.length > 0;
    if (!selectedResource && hasResourcesWithAgents) {
      resourcesNav.set(resourceOptionId(resources[0]));
    }
  }, [resources, selectedResource, resourcesNav]);

  const select = useCallback(
    (resource: WorkspaceResource) => {
      resourcesNav.set(resourceOptionId(resource));
    },
    [resourcesNav],
  );

  const isSelected = useCallback(
    (resource: WorkspaceResource) => {
      return resourceOptionId(resource) === resourcesNav.value;
    },
    [resourcesNav.value],
  );

  return {
    isSelected,
    select,
    selected: selectedResource,
  };
};
