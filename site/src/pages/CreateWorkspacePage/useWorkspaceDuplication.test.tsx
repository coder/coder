import { act, waitFor } from "@testing-library/react";
import * as M from "../../testHelpers/entities";
import { type Workspace } from "api/typesGenerated";
import { useWorkspaceDuplication } from "./useWorkspaceDuplication";
import { MockWorkspace } from "testHelpers/entities";
import CreateWorkspacePage from "./CreateWorkspacePage";
import {
  type GetLocationSnapshot,
  renderHookWithAuth,
} from "testHelpers/renderHelpers";

function render(workspace?: Workspace) {
  return renderHookWithAuth(
    ({ workspace }) => useWorkspaceDuplication(workspace),
    {
      renderOptions: { initialProps: { workspace } },
      routingOptions: {
        extraRoutes: [
          {
            path: "/templates/:template/workspace",
            element: <CreateWorkspacePage />,
          },
        ],
      },
    },
  );
}

type RenderResult = Awaited<ReturnType<typeof render>>;
type RenderHookResult = RenderResult["result"];

async function performNavigation(
  result: RenderHookResult,
  getSnapshot: GetLocationSnapshot,
) {
  await waitFor(() => expect(result.current.isDuplicationReady).toBe(true));
  void act(() => result.current.duplicateWorkspace());

  const templateName = MockWorkspace.template_name;
  return waitFor(() => {
    const { pathname } = getSnapshot();
    expect(pathname).toEqual(`/templates/${templateName}/workspace`);
  });
}

describe(`${useWorkspaceDuplication.name}`, () => {
  it("Will never be ready when there is no workspace passed in", async () => {
    const { result, rerender } = await render(undefined);
    expect(result.current.isDuplicationReady).toBe(false);

    for (let i = 0; i < 10; i++) {
      rerender({ workspace: undefined });
      expect(result.current.isDuplicationReady).toBe(false);
    }
  });

  it("Will become ready when workspace is provided and build params are successfully fetched", async () => {
    const { result } = await render(MockWorkspace);
    expect(result.current.isDuplicationReady).toBe(false);
    await waitFor(() => expect(result.current.isDuplicationReady).toBe(true));
  });

  it("Is able to navigate the user to the workspace creation page", async () => {
    const { result, getLocationSnapshot } = await render(MockWorkspace);
    await performNavigation(result, getLocationSnapshot);
  });

  test("Navigating populates the URL search params with the workspace's build params", async () => {
    const { result, getLocationSnapshot } = await render(MockWorkspace);
    await performNavigation(result, getLocationSnapshot);

    const { search } = getLocationSnapshot();
    const parsedParams = new URLSearchParams(search);

    const mockBuildParams = [
      M.MockWorkspaceBuildParameter1,
      M.MockWorkspaceBuildParameter2,
      M.MockWorkspaceBuildParameter3,
      M.MockWorkspaceBuildParameter4,
      M.MockWorkspaceBuildParameter5,
    ];

    for (const { name, value } of mockBuildParams) {
      const key = `param.${name}`;
      expect(parsedParams.get(key)).toEqual(value);
    }
  });

  test("Navigating appends other necessary metadata to the search params", async () => {
    const { result, getLocationSnapshot } = await render(MockWorkspace);
    await performNavigation(result, getLocationSnapshot);

    const { search } = getLocationSnapshot();
    const parsedParams = new URLSearchParams(search);

    const extraMetadataEntries = [
      ["mode", "duplicate"],
      ["name", `${MockWorkspace.name}-copy`],
      ["version", MockWorkspace.template_active_version_id],
    ] as const;

    for (const [key, value] of extraMetadataEntries) {
      expect(parsedParams.get(key)).toBe(value);
    }
  });
});
