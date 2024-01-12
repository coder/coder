import { WorkspaceResource } from "api/typesGenerated";
import { useTab } from "hooks";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useCallback, useEffect, useMemo } from "react";

export const resourceOptionId = (resource: WorkspaceResource) => {
  return `${resource.type}_${resource.name}`;
};

export const useResourcesNav = (resources: WorkspaceResource[]) => {
  const firstResource = useMemo(
    () => (resources.length > 0 ? resources[0] : undefined),
    [resources],
  );
  const resourcesNav = useTab(
    "resources",
    firstResource ? resourceOptionId(firstResource) : "",
  );

  const isSelected = useCallback(
    (resource: WorkspaceResource) => {
      return resourceOptionId(resource) === resourcesNav.value;
    },
    [resourcesNav.value],
  );

  const selectedResource = resources.find(isSelected);
  const onSelectedResourceChange = useEffectEvent(
    (previousResource?: WorkspaceResource) => {
      const hasResourcesWithAgents =
        firstResource &&
        firstResource.agents &&
        firstResource.agents.length > 0;
      if (!previousResource && hasResourcesWithAgents) {
        resourcesNav.set(resourceOptionId(firstResource));
      }
    },
  );
  useEffect(() => {
    onSelectedResourceChange(selectedResource);
  }, [onSelectedResourceChange, selectedResource]);

  const select = useCallback(
    (resource: WorkspaceResource) => {
      resourcesNav.set(resourceOptionId(resource));
    },
    [resourcesNav],
  );

  return {
    isSelected,
    select,
    selected: selectedResource,
  };
};
