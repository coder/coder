import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
import { AgentRow, AgentRowProps } from "./AgentRow";
import { DisplayAppNameMap } from "./AppLink/AppLink";
import { screen } from "@testing-library/react";
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";

jest.mock("components/Resources/AgentMetadata", () => {
  const AgentMetadata = () => <></>;
  return { AgentMetadata };
});

describe.each<{
  result: "visible" | "hidden";
  props: Partial<AgentRowProps>;
}>([
  {
    result: "visible",
    props: {
      showApps: true,
      agent: {
        ...MockWorkspaceAgent,
        display_apps: ["vscode", "vscode_insiders"],
        status: "connected",
      },
      hideVSCodeDesktopButton: false,
    },
  },
  {
    result: "hidden",
    props: {
      showApps: false,
      agent: {
        ...MockWorkspaceAgent,
        display_apps: ["vscode", "vscode_insiders"],
        status: "connected",
      },
      hideVSCodeDesktopButton: false,
    },
  },
  {
    result: "hidden",
    props: {
      showApps: true,
      agent: {
        ...MockWorkspaceAgent,
        display_apps: [],
        status: "connected",
      },
      hideVSCodeDesktopButton: false,
    },
  },
  {
    result: "hidden",
    props: {
      showApps: true,
      agent: {
        ...MockWorkspaceAgent,
        display_apps: ["vscode", "vscode_insiders"],
        status: "disconnected",
      },
      hideVSCodeDesktopButton: false,
    },
  },
  {
    result: "hidden",
    props: {
      showApps: true,
      agent: {
        ...MockWorkspaceAgent,
        display_apps: ["vscode", "vscode_insiders"],
        status: "connected",
      },
      hideVSCodeDesktopButton: true,
    },
  },
])("VSCode button visibility", ({ props: testProps, result }) => {
  const props: AgentRowProps = {
    agent: MockWorkspaceAgent,
    workspace: MockWorkspace,
    showApps: false,
    serverVersion: "",
    serverAPIVersion: "",
    onUpdateAgent: function (): void {
      throw new Error("Function not implemented.");
    },
    ...testProps,
  };

  test(`visibility: ${result}, showApps: ${props.showApps}, hideVSCodeDesktopButton: ${props.hideVSCodeDesktopButton}, display apps: ${props.agent.display_apps}`, async () => {
    renderWithAuth(<AgentRow {...props} />);
    await waitForLoaderToBeRemoved();

    if (result === "visible") {
      expect(screen.getByText(DisplayAppNameMap["vscode"])).toBeVisible();
    } else {
      expect(screen.queryByText(DisplayAppNameMap["vscode"])).toBeNull();
    }
  });
});
