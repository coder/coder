import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
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

const listeningPortsWithSubstringMatch = [
	...MockListeningPortsResponse.ports,
	{ process_name: "substring-match", network: "", port: 18080 },
];

const typeInPortFilter = (canvasElement: HTMLElement, text: string) =>
	userEvent.type(
		within(canvasElement).getByRole("textbox", { name: "Filter ports" }),
		text,
	);

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
	play: async ({ canvasElement }) => {
		await typeInPortFilter(canvasElement, "808");
	},
};

export const NoMatchingPorts: Story = {
	play: async ({ canvasElement }) => {
		await typeInPortFilter(canvasElement, "1234");
	},
};

export const InvalidPort: Story = {
	play: async ({ canvasElement }) => {
		await typeInPortFilter(canvasElement, "5");
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
