import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import {
	MockListeningPortsResponse,
	MockSharedPortsResponse,
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
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

export const WithPorts: Story = {
	args: {
		listeningPorts: MockListeningPortsResponse.ports,
		sharedPorts: MockSharedPortsResponse.shares,
	},
};

export const FilterPorts: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const portTrigger = canvas.getByRole("button", {
			name: "Open port picker",
		});

		await userEvent.click(portTrigger);
		const portInput = within(screen.getByRole("dialog")).getByRole("combobox");
		await waitFor(() =>
			expect(screen.getByRole("option", { name: /30000/ })).toBeVisible(),
		);
		await expect(screen.getByRole("option", { name: /8080/ })).toBeVisible();

		await userEvent.type(portInput, "808");
		await expect(screen.getByRole("option", { name: /8080/ })).toBeVisible();
		await expect(
			screen.queryByRole("option", { name: /30000/ }),
		).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole("option", { name: /8080/ }));
		await expect(portTrigger).toHaveTextContent("8080");
		await expect(canvas.getByRole("link", { name: "8080" })).toBeVisible();
		await expect(canvas.getByRole("link", { name: "4000" })).toBeVisible();

		await userEvent.click(portTrigger);
		const customPortInput = within(screen.getByRole("dialog")).getByRole(
			"combobox",
		);
		await userEvent.type(customPortInput, "9999");

		await expect(
			within(screen.getByRole("dialog")).getByText(
				"Press Enter to use this port.",
			),
		).toBeVisible();
		await userEvent.keyboard("{Enter}");
		await expect(portTrigger).toHaveTextContent("9999");
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
};

export const Empty: Story = {
	args: {
		listeningPorts: [],
		sharedPorts: [],
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
