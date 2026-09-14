import { screen } from "@testing-library/react";
import {
	MockListeningPortsResponse,
	MockSharedPortsResponse,
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { PortForwardPopoverView } from "./PortForwardButton";

describe("PortForwardPopoverView", () => {
	it("adds an accessible name to each shared-port delete button", () => {
		render(
			<PortForwardPopoverView
				host="coder.test"
				workspace={MockWorkspace}
				agent={MockWorkspaceAgent}
				template={MockTemplate}
				sharedPorts={MockSharedPortsResponse.shares}
				listeningPorts={MockListeningPortsResponse.ports}
				portSharingControlsEnabled
				refetchSharedPorts={vi.fn()}
			/>,
		);

		expect(
			screen.getAllByRole("button", { name: "Delete shared port" }),
		).toHaveLength(MockSharedPortsResponse.shares.length);
	});
});
