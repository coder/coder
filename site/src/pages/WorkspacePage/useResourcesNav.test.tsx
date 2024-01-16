import { renderHook } from "@testing-library/react";
import { resourceOptionValue, useResourcesNav } from "./useResourcesNav";
import { WorkspaceResource } from "api/typesGenerated";
import { MockWorkspaceResource } from "testHelpers/entities";
import { RouterProvider, createMemoryRouter } from "react-router-dom";

describe("useResourcesNav", () => {
  it("selects the first resource if it has agents and no resource is selected", () => {
    const resources: WorkspaceResource[] = [
      MockWorkspaceResource,
      {
        ...MockWorkspaceResource,
        agents: [],
      },
    ];
    const { result } = renderHook(() => useResourcesNav(resources), {
      wrapper: ({ children }) => (
        <RouterProvider
          router={createMemoryRouter([{ path: "/", element: children }])}
        />
      ),
    });
    expect(result.current.value).toBe(
      resourceOptionValue(MockWorkspaceResource),
    );
  });

  it("selects the resource passed in the URL", () => {
    const resources: WorkspaceResource[] = [
      {
        ...MockWorkspaceResource,
        type: "docker_container",
        name: "coder_python",
      },
      {
        ...MockWorkspaceResource,
        type: "docker_container",
        name: "coder_java",
      },
      {
        ...MockWorkspaceResource,
        type: "docker_image",
        name: "coder_image_python",
        agents: [],
      },
    ];
    const { result } = renderHook(() => useResourcesNav(resources), {
      wrapper: ({ children }) => (
        <RouterProvider
          router={createMemoryRouter([{ path: "/", element: children }], {
            initialEntries: [
              `/?resources=${resourceOptionValue(resources[1])}`,
            ],
          })}
        />
      ),
    });
    expect(result.current.value).toBe(resourceOptionValue(resources[1]));
  });

  it("selects a resource when resources are updated", () => {
    const startedResources: WorkspaceResource[] = [
      {
        ...MockWorkspaceResource,
        type: "docker_container",
        name: "coder_python",
      },
      {
        ...MockWorkspaceResource,
        type: "docker_container",
        name: "coder_java",
      },
      {
        ...MockWorkspaceResource,
        type: "docker_image",
        name: "coder_image_python",
        agents: [],
      },
    ];
    const { result, rerender } = renderHook(
      ({ resources }) => useResourcesNav(resources),
      {
        wrapper: ({ children }) => (
          <RouterProvider
            router={createMemoryRouter([{ path: "/", element: children }])}
          />
        ),
        initialProps: { resources: startedResources },
      },
    );
    expect(result.current.value).toBe(resourceOptionValue(startedResources[0]));

    // When a workspace is stopped, there are no resources with agents, so we
    // need to retain the currently selected resource. This ensures consistency
    // when handling workspace updates that involve a sequence of stopping and
    // starting. By preserving the resource selection, we maintain the desired
    // configuration and prevent unintended changes during the stop-and-start
    // process.
    const stoppedResources: WorkspaceResource[] = [
      {
        ...MockWorkspaceResource,
        type: "docker_image",
        name: "coder_image_python",
        agents: [],
      },
    ];
    rerender({ resources: stoppedResources });
    expect(result.current.value).toBe(resourceOptionValue(startedResources[0]));

    // When a workspace is started again a resource is selected
    rerender({ resources: startedResources });
    expect(result.current.value).toBe(resourceOptionValue(startedResources[0]));
  });

  // This happens when a new workspace is created and there are no resources
  it("selects a resource when resources are not defined previously", () => {
    const startingResources: WorkspaceResource[] = [];
    const { result, rerender } = renderHook(
      ({ resources }) => useResourcesNav(resources),
      {
        wrapper: ({ children }) => (
          <RouterProvider
            router={createMemoryRouter([{ path: "/", element: children }])}
          />
        ),
        initialProps: { resources: startingResources },
      },
    );
    const startedResources: WorkspaceResource[] = [
      {
        ...MockWorkspaceResource,
        type: "docker_container",
        name: "coder_python",
      },
      {
        ...MockWorkspaceResource,
        type: "docker_container",
        name: "coder_java",
      },
      {
        ...MockWorkspaceResource,
        type: "docker_image",
        name: "coder_image_python",
        agents: [],
      },
    ];
    rerender({ resources: startedResources });
    expect(result.current.value).toBe(resourceOptionValue(startedResources[0]));
  });
});
