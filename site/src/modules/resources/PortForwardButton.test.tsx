import { screen } from "@testing-library/react";
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

const portField = () =>
	screen.getByRole("combobox", { name: "Connect to port" });

const connectButton = () => screen.getByRole("button", { name: "Connect" });

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

		await userEvent.click(portField());
		await userEvent.click(screen.getByRole("option", { name: /8080/ }));
		await userEvent.click(connectButton());

		expect(open).toHaveBeenCalledWith(expectedURL(8080), "_blank");
	});

	it("opens a custom port and strips leading zeros", async () => {
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		renderPopover();

		await userEvent.type(portField(), "09999");
		await userEvent.click(
			screen.getByRole("option", { name: "Use port 9999" }),
		);
		await userEvent.click(connectButton());

		expect(open).toHaveBeenCalledWith(expectedURL(9999), "_blank");
	});

	it("connects to a typed port with the keyboard alone", async () => {
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		renderPopover();

		await userEvent.type(portField(), "9999{Enter}{Enter}");

		expect(open).toHaveBeenCalledWith(expectedURL(9999), "_blank");
	});

	it("does not connect while the typed port is out of range", async () => {
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		renderPopover();

		await userEvent.type(portField(), "99999");
		expect(connectButton()).toBeDisabled();

		expect(open).not.toHaveBeenCalled();
	});
});
