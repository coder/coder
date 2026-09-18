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

const HOST = "*.example.com";

const renderPopover = () =>
	render(
		<PortForwardPopoverView
			host={HOST}
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
		HOST,
		port,
		MockWorkspaceAgent.name,
		MockWorkspace.name,
		MockWorkspace.owner_name,
		getWorkspaceListeningPortsProtocol(MockWorkspace.id),
	);

const openPortPicker = async (triggerName = "Connect to port...") => {
	await userEvent.click(screen.getByRole("button", { name: triggerName }));
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

	it("keeps the port selected when it is picked again", async () => {
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		renderPopover();

		const dialog = await openPortPicker();
		await userEvent.click(within(dialog).getByRole("option", { name: /8080/ }));
		const reopened = await openPortPicker("Connect to port 8080");
		await userEvent.click(
			within(reopened).getByRole("option", { name: /8080/ }),
		);
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
