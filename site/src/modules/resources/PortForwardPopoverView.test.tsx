import { screen } from "@testing-library/react";
import { QueryClientProvider, QueryClient } from "react-query";
import {
  MockListeningPortsResponse,
  MockTemplate,
  MockWorkspace,
  MockWorkspaceAgent,
} from "testHelpers/entities";
import { renderComponent } from "testHelpers/renderHelpers";
import { PortForwardPopoverView } from "./PortForwardButton";

describe("Port Forward Popover View", () => {
  it("renders component", async () => {
    renderComponent(
      <QueryClientProvider client={new QueryClient()}>
        <PortForwardPopoverView
          agent={MockWorkspaceAgent}
          template={MockTemplate}
          workspaceID={MockWorkspace.id}
          listeningPorts={MockListeningPortsResponse.ports}
          portSharingExperimentEnabled
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
