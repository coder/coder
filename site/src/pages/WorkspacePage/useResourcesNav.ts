import { WorkspaceResource } from "api/typesGenerated";
import { useTab } from "hooks";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useCallback, useEffect } from "react";

export const resourceOptionId = (resource: WorkspaceResource) => {
  return `${resource.type}_${resource.name}`;
};

// TODO: This currently serves as a temporary workaround for synchronizing the
// resources tab during workspace transitions. The optimal resolution involves
// eliminating the sync and updating the URL within the workspace data update
// event in the WebSocket "onData" event. However, this requires substantial
// refactoring. Consider revisiting this solution in the future for a more
// robust implementation.
export const useResourcesNav = (resources: WorkspaceResource[]) => {
  const resourcesNav = useTab("resources", "");

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
        resources.length > 0 &&
        resources[0].agents &&
        resources[0].agents.length > 0;
      if (!previousResource && hasResourcesWithAgents) {
        resourcesNav.set(resourceOptionId(resources[0]));
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
    selectedValue: resourcesNav.value,
  };
};
