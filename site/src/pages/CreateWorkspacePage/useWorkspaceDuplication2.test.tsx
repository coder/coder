import { act, waitFor } from "@testing-library/react";
// import * as M from "../../testHelpers/entities";
import { type Workspace } from "api/typesGenerated";
import { useWorkspaceDuplication } from "./useWorkspaceDuplication";
import { MockWorkspace } from "testHelpers/entities";
import CreateWorkspacePage from "./CreateWorkspacePage";
import { renderHookWithAuth } from "testHelpers/renderHelpers";

function getPathName(content: string) {
  return `/templates/${content}/workspace` as const;
}

function render(workspace?: Workspace) {
  return renderHookWithAuth(
    ({ workspace }) => useWorkspaceDuplication(workspace),
    {
      renderOptions: { initialProps: { workspace } },
      routingOptions: {
        extraRoutes: [
          {
            path: getPathName(":template"),
            element: <CreateWorkspacePage />,
          },
        ],
      },
    },
  );
}

type RenderResult = Awaited<ReturnType<typeof render>>;
type RenderHookResult = RenderResult["result"];
type GetLocationSnapshot = RenderResult["getLocationSnapshot"];

async function performNavigation(
  result: RenderHookResult,
  getSnapshot: GetLocationSnapshot,
) {
  await waitFor(() => expect(result.current.isDuplicationReady).toBe(true));
  void act(() => result.current.duplicateWorkspace());

  return waitFor(() => {
    const { pathname } = getSnapshot();
    expect(pathname).toEqual(getPathName(MockWorkspace.template_name));
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

  test.skip("Navigating populates the URL search params with the workspace's build params", async () => {
    // const { result, router } = await render(MockWorkspace);
    // await performNavigation(result, router);
    // const parsedParams = new URLSearchParams(router.state.location.search);
    // const mockBuildParams = [
    //   M.MockWorkspaceBuildParameter1,
    //   M.MockWorkspaceBuildParameter2,
    //   M.MockWorkspaceBuildParameter3,
    //   M.MockWorkspaceBuildParameter4,
    //   M.MockWorkspaceBuildParameter5,
    // ];
    // for (const { name, value } of mockBuildParams) {
    //   const key = `param.${name}`;
    //   expect(parsedParams.get(key)).toEqual(value);
    // }
  });

  test.skip("Navigating appends other necessary metadata to the search params", async () => {
    // const { result, router } = await render(MockWorkspace);
    // await performNavigation(result, router);
    // const parsedParams = new URLSearchParams(router.state.location.search);
    // const extraMetadataEntries = [
    //   ["mode", "duplicate"],
    //   ["name", `${MockWorkspace.name}-copy`],
    //   ["version", MockWorkspace.template_active_version_id],
    // ] as const;
    // for (const [key, value] of extraMetadataEntries) {
    //   expect(parsedParams.get(key)).toBe(value);
    // }
  });
});
