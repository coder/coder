import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import {
	MockListeningPortsResponse,
	MockSharedPortsResponse,
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import {
	getWorkspaceListeningPortsProtocol,
	portForwardURL,
} from "#/utils/portForward";
import { PortForwardPopoverView } from "./PortForwardButton";

const meta: Meta<typeof PortForwardPopoverView> = {
	title: "modules/resources/PortForwardPopoverView",
	component: PortForwardPopoverView,
	decorators: [
		(Story) => (
			<div className="w-[404px] rounded-lg border border-solid border-border bg-surface-primary">
				<Story />
			</div>
		),
	],
	args: {
		listeningPorts: MockListeningPortsResponse.ports,
		sharedPorts: MockSharedPortsResponse.shares,
		agent: MockWorkspaceAgent,
		template: MockTemplate,
		workspace: MockWorkspace,
		portSharingControlsEnabled: true,
		host: "*.coder.com",
		refetchSharedPorts: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof PortForwardPopoverView>;

const listeningPortsWithSubstringMatch = [
	...MockListeningPortsResponse.ports,
	{ process_name: "substring-match", network: "", port: 19999 },
];

export const WithPorts: Story = {
	args: {
		listeningPorts: MockListeningPortsResponse.ports,
		sharedPorts: MockSharedPortsResponse.shares,
	},
};

export const FilterPorts: Story = {
	args: {
		listeningPorts: listeningPortsWithSubstringMatch,
		sharedPorts: MockSharedPortsResponse.shares.filter(
			(share) => share.port !== 8081,
		),
	},
	beforeEach: () => {
		const open = spyOn(window, "open").mockReturnValue(null);
		return () => open.mockRestore();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const portTrigger = canvas.getByRole("button", {
			name: "Connect to port...",
		});
		const submitButton = canvas.getByRole("button", {
			name: "Connect to selected port",
		});
		await userEvent.click(submitButton);
		const portDialog = screen.getByRole("dialog", { name: "Port picker" });
		const portInput = within(portDialog).getByRole("combobox", {
			name: "Filter or enter port",
		});
		await waitFor(() => expect(portInput).toHaveFocus());
		await expect(portInput).toHaveAttribute("inputmode", "numeric");
		await waitFor(() =>
			expect(
				within(portDialog).getByRole("option", { name: /30000/ }),
			).toBeVisible(),
		);

		await userEvent.type(portInput, "808");
		await expect(
			within(portDialog).getByRole("option", { name: /8080/ }),
		).toBeVisible();
		await expect(
			within(portDialog).getByRole("option", { name: /8081/ }),
		).toBeVisible();
		await expect(
			within(portDialog).getByRole("option", { name: "Use port 808" }),
		).toBeVisible();
		await expect(
			within(portDialog).queryByRole("option", { name: /30000/ }),
		).not.toBeInTheDocument();

		await userEvent.keyboard("{Enter}");
		await expect(portTrigger).toHaveTextContent("8080");
		await expect(portTrigger).toHaveAccessibleName("Connect to port 8080");
		await expect(submitButton).toBeEnabled();
		await waitFor(() => expect(submitButton).toHaveFocus());
		await expect(canvas.getByRole("link", { name: "8080" })).toBeVisible();
		await expect(canvas.getByRole("link", { name: "4000" })).toBeVisible();

		await userEvent.keyboard("{Enter}");
		await expect(window.open).toHaveBeenCalledWith(
			portForwardURL(
				"*.coder.com",
				8080,
				MockWorkspaceAgent.name,
				MockWorkspace.name,
				MockWorkspace.owner_name,
				getWorkspaceListeningPortsProtocol(MockWorkspace.id),
			),
			"_blank",
		);
		await expect(window.open).toHaveBeenCalledTimes(1);

		await userEvent.click(portTrigger);
		const reselectDialog = screen.getByRole("dialog", { name: "Port picker" });
		await userEvent.click(
			within(reselectDialog).getByRole("option", { name: /8080/ }),
		);
		await expect(portTrigger).toHaveTextContent("8080");
		await expect(submitButton).toBeEnabled();
		await waitFor(() => expect(submitButton).toHaveFocus());

		await userEvent.click(portTrigger);
		const customPortDialog = screen.getByRole("dialog", {
			name: "Port picker",
		});
		const customPortInput = within(customPortDialog).getByRole("combobox", {
			name: "Filter or enter port",
		});
		await userEvent.type(customPortInput, "09999");
		await expect(customPortInput).toHaveValue("9999");
		await expect(
			within(customPortDialog).getByRole("option", {
				name: "Use port 9999",
			}),
		).toBeVisible();
		await expect(
			within(customPortDialog).getByRole("option", { name: /19999/ }),
		).toBeVisible();
		await userEvent.keyboard("{Enter}");
		await expect(portTrigger).toHaveTextContent("9999");
		await waitFor(() => expect(submitButton).toHaveFocus());
	},
};

export const WithManyPorts: Story = {
	args: {
		listeningPorts: Array.from({ length: 20 }).map((_, i) => ({
			process_name: `port-${i}`,
			network: "",
			port: 3000 + i,
		})),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const portTrigger = canvas.getByRole("button", {
			name: "Connect to port...",
		});

		await userEvent.click(portTrigger);
		const portDialog = screen.getByRole("dialog", { name: "Port picker" });
		const portInput = within(portDialog).getByRole("combobox", {
			name: "Filter or enter port",
		});
		await expect(portInput).toHaveAttribute("inputmode", "numeric");
		await waitFor(() =>
			expect(
				within(portDialog).getByRole("option", { name: /3019/ }),
			).toBeVisible(),
		);
		await userEvent.type(portInput, "3019");
		await userEvent.keyboard("{Enter}");
		await expect(portTrigger).toHaveTextContent("3019");
	},
};

export const Empty: Story = {
	args: {
		listeningPorts: [],
		sharedPorts: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Connect to port..." }),
		);
		const portDialog = screen.getByRole("dialog", { name: "Port picker" });
		const portInput = within(portDialog).getByRole("combobox", {
			name: "Filter or enter port",
		});
		await expect(portInput).toHaveAttribute("inputmode", "numeric");
		await waitFor(() =>
			expect(
				within(portDialog).getByText("Enter a port number to connect."),
			).toBeVisible(),
		);

		await userEvent.type(portInput, "5");
		await expect(
			within(portDialog).getByText("Enter a port from 9 to 65535."),
		).toBeVisible();
		await expect(
			within(portDialog).queryByRole("option", { name: "Use port 5" }),
		).not.toBeInTheDocument();
	},
};

export const AGPLPortSharing: Story = {
	args: {
		portSharingControlsEnabled: false,
		sharedPorts: MockSharedPortsResponse.shares,
	},
};

export const EnterprisePortSharingControlsOwner: Story = {
	args: {
		template: {
			...MockTemplate,
			max_port_share_level: "owner",
		},
	},
};

export const EnterprisePortSharingControlsAuthenticated: Story = {
	args: {
		template: {
			...MockTemplate,
			max_port_share_level: "authenticated",
		},
		sharedPorts: MockSharedPortsResponse.shares.filter(
			(share) => share.share_level === "authenticated",
		),
	},
};

export const DisabledOptions: Story = {
	args: {
		template: {
			...MockTemplate,
			max_port_share_level: "organization",
		},
		sharedPorts: MockSharedPortsResponse.shares.filter(
			(share) => share.share_level === "organization",
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const dropdown = canvas.getByLabelText("Sharing Level");
		await userEvent.click(dropdown);
	},
};
