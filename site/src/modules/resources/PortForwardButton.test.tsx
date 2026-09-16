import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	MockListeningPortsResponse,
	MockSharedPortsResponse,
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import {
	getWorkspaceListeningPortsProtocol,
	portForwardURL,
} from "#/utils/portForward";
import { PortForwardPopoverView } from "./PortForwardButton";

const host = "*.coder.com";

const renderPopover = () =>
	render(
		<PortForwardPopoverView
			host={host}
			workspace={MockWorkspace}
			agent={MockWorkspaceAgent}
			template={MockTemplate}
			sharedPorts={MockSharedPortsResponse.shares}
			listeningPorts={MockListeningPortsResponse.ports}
			portSharingControlsEnabled
			refetchSharedPorts={vi.fn()}
		/>,
	);

const expectedURL = (port: number) =>
	portForwardURL(
		host,
		port,
		MockWorkspaceAgent.name,
		MockWorkspace.name,
		MockWorkspace.owner_name,
		getWorkspaceListeningPortsProtocol(MockWorkspace.id),
	);

const openPortPicker = async () => {
	await userEvent.click(
		screen.getByRole("button", { name: "Connect to port..." }),
	);
	return screen.getByRole("dialog", { name: "Port picker" });
};

describe("PortForwardPopoverView", () => {
	it("adds an accessible name to each shared-port delete button", () => {
		renderPopover();

		expect(
			screen.getAllByRole("button", { name: "Delete shared port" }),
		).toHaveLength(MockSharedPortsResponse.shares.length);
	});

	it("opens the selected listening port in a new tab", async () => {
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		renderPopover();

		const dialog = await openPortPicker();
		await userEvent.click(within(dialog).getByRole("option", { name: /8080/ }));
		await userEvent.click(
			screen.getByRole("button", { name: "Connect to selected port" }),
		);

		expect(open).toHaveBeenCalledWith(expectedURL(8080), "_blank");
	});

	it("opens a custom port and strips leading zeros", async () => {
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		renderPopover();

		const dialog = await openPortPicker();
		await userEvent.type(
			within(dialog).getByRole("combobox", { name: "Filter or enter port" }),
			"09999",
		);
		await userEvent.click(
			within(dialog).getByRole("option", { name: "Use port 9999" }),
		);
		await userEvent.click(
			screen.getByRole("button", { name: "Connect to selected port" }),
		);

		expect(open).toHaveBeenCalledWith(expectedURL(9999), "_blank");
	});
});
