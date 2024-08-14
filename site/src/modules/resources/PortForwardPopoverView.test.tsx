import { screen } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import {
  MockListeningPortsResponse,
  MockTemplate,
  MockWorkspace,
  MockWorkspaceAgent,
} from "testHelpers/entities";
import {
  renderComponent,
  createTestQueryClient,
} from "testHelpers/renderHelpers";
import { PortForwardPopoverView } from "./PortForwardButton";

describe("Port Forward Popover View", () => {
  it("renders component", async () => {
    renderComponent(
      <QueryClientProvider client={createTestQueryClient()}>
        <PortForwardPopoverView
          agent={MockWorkspaceAgent}
          template={MockTemplate}
          workspaceID={MockWorkspace.id}
          listeningPorts={MockListeningPortsResponse.ports}
          portSharingControlsEnabled
          host="host"
          username="username"
          workspaceName="workspaceName"
        />
      </QueryClientProvider>,
    );

    expect(
      screen.getByText(MockListeningPortsResponse.ports[0].port),
    ).toBeInTheDocument();

    expect(
      screen.getByText(MockListeningPortsResponse.ports[0].process_name),
    ).toBeInTheDocument();
  });
});
