import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
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

const renderPopover = (template = MockTemplate) =>
	render(
		<PortForwardPopoverView
			host={HOST}
			workspace={MockWorkspace}
			agent={MockWorkspaceAgent}
			template={template}
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
		MockWorkspace.id,
		getWorkspaceListeningPortsProtocol(MockWorkspace.id),
	);

const typePort = (text: string) =>
	userEvent.type(
		within(screen.getByRole("group", { name: "Listening ports" })).getByRole(
			"textbox",
			{ name: "Port" },
		),
		text,
	);

describe("PortForwardPopoverView", () => {
	it("names each listening-port link by its port and process", () => {
		renderPopover();

		for (const { port, process_name } of [
			{ port: 30000, process_name: "webb" },
			{ port: 8080, process_name: "gogo" },
		]) {
			expect(
				screen.getByRole("link", { name: `${port} ${process_name}` }),
			).toHaveAttribute("href", expectedURL(port));
		}
	});

	it("shares the port named by the row's share button", async () => {
		const upsert = vi
			.spyOn(API, "upsertWorkspaceAgentSharedPort")
			.mockResolvedValue(MockSharedPortsResponse);
		renderPopover();

		await userEvent.click(
			screen.getByRole("button", { name: "Share port 8080" }),
		);

		expect(upsert).toHaveBeenCalledWith(
			MockWorkspace.id,
			expect.objectContaining({ port: 8080 }),
		);
	});

	it("deletes the port named by the row's delete button", async () => {
		const remove = vi
			.spyOn(API, "deleteWorkspaceAgentSharedPort")
			.mockResolvedValue({ shares: [] });
		renderPopover();

		await userEvent.click(
			screen.getByRole("button", { name: "Delete shared port 4443" }),
		);

		expect(remove).toHaveBeenCalledWith(MockWorkspace.id, {
			agent_name: MockWorkspaceAgent.name,
			port: 4443,
		});
	});

	it("describes why sharing levels are unavailable", () => {
		renderPopover({ ...MockTemplate, max_port_share_level: "organization" });
		const reason =
			"This workspace template does not allow sharing ports outside of its organization.";

		expect(
			screen.getByRole("combobox", { name: "Sharing level" }),
		).toHaveAccessibleDescription(reason);
		expect(
			screen.getByRole("combobox", { name: "Sharing level for port 4443" }),
		).toHaveAccessibleDescription(reason);
	});

	it("opens the typed port in a new tab, ignoring leading zeros and whitespace", async () => {
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		renderPopover();

		await typePort(" 09999 {enter}");

		expect(open).toHaveBeenCalledWith(expectedURL(9999), "_blank");
	});

	it("does not connect when the text is not only a port number", async () => {
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		renderPopover();

		await typePort("8080abc{enter}");
		await userEvent.click(screen.getByRole("button", { name: "Connect" }));

		expect(open).not.toHaveBeenCalled();
	});

	it("does not connect to a port outside the valid range", async () => {
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		renderPopover();

		await typePort("5{enter}");
		await userEvent.click(screen.getByRole("button", { name: "Connect" }));

		expect(open).not.toHaveBeenCalled();
	});
});
