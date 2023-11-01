import { waitFor, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router-dom";
import { renderWithRouter } from "testHelpers/renderHelpers";

import * as M from "../../testHelpers/entities";
import { type Workspace } from "api/typesGenerated";
import { useWorkspaceDuplication } from "./useWorkspaceDuplication";
import { MockWorkspace } from "testHelpers/entities";
import CreateWorkspacePage from "./CreateWorkspacePage";

// Tried really hard to get these tests working with RTL's renderHook, but I
// kept running into weird function mismatches, mostly stemming from the fact
// that React Router's RouteProvider does not accept children, meaning that you
// can't inject values into it with renderHook's wrapper
function WorkspaceMock({ workspace }: { workspace?: Workspace }) {
  const { duplicateWorkspace, isDuplicationReady } =
    useWorkspaceDuplication(workspace);

  return (
    <button onClick={duplicateWorkspace} disabled={!isDuplicationReady}>
      Click me!
    </button>
  );
}

function renderInMemory(workspace?: Workspace) {
  const router = createMemoryRouter([
    { path: "/", element: <WorkspaceMock workspace={workspace} /> },
    {
      path: "/templates/:template/workspace",
      element: <CreateWorkspacePage />,
    },
  ]);

  return renderWithRouter(router);
}

async function performNavigation(
  button: HTMLElement,
  router: ReturnType<typeof createMemoryRouter>,
) {
  await waitFor(() => expect(button).not.toBeDisabled());

  await userEvent.click(button);
  await waitFor(() => {
    expect(router.state.location.pathname).toEqual(
      `/templates/${MockWorkspace.template_name}/workspace`,
    );
  });
}

describe(`${useWorkspaceDuplication.name}`, () => {
  it("Will never be ready when there is no workspace passed in", async () => {
    const { rootComponent, rerender } = renderInMemory(undefined);
    const button = await screen.findByRole("button");
    expect(button).toBeDisabled();

    for (let i = 0; i < 10; i++) {
      rerender(rootComponent);
      expect(button).toBeDisabled();
    }
  });

  it("Will become ready when workspace is provided and build params are successfully fetched", async () => {
    renderInMemory(MockWorkspace);
    const button = await screen.findByRole("button");

    expect(button).toBeDisabled();
    await waitFor(() => expect(button).not.toBeDisabled());
  });

  it("duplicateWorkspace navigates the user to the workspace creation page", async () => {
    const { router } = renderInMemory(MockWorkspace);
    const button = await screen.findByRole("button");
    await performNavigation(button, router);
  });

  test("Navigating populates the URL search params with the workspace's build params", async () => {
    const { router } = renderInMemory(MockWorkspace);
    const button = await screen.findByRole("button");
    await performNavigation(button, router);

    const parsedParams = new URLSearchParams(router.state.location.search);
    const mockBuildParams = [
      M.MockWorkspaceBuildParameter1,
      M.MockWorkspaceBuildParameter2,
      M.MockWorkspaceBuildParameter3,
      M.MockWorkspaceBuildParameter4,
      M.MockWorkspaceBuildParameter5,
    ];

    for (const p of mockBuildParams) {
      expect(parsedParams.get(`param.${p.name}`)).toEqual(p.value);
    }
  });

  test("Navigating appends other necessary metadata to the search params", async () => {
    const { router } = renderInMemory(MockWorkspace);
    const button = await screen.findByRole("button");
    await performNavigation(button, router);

    const parsedParams = new URLSearchParams(router.state.location.search);
    const expectedParamEntries = [
      ["mode", "duplicate"],
      ["name", MockWorkspace.name],
      ["version", MockWorkspace.template_active_version_id],
    ] as const;

    for (const [key, value] of expectedParamEntries) {
      expect(parsedParams.get(key)).toBe(value);
    }
  });
});
