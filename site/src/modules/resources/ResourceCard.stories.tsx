import { MockWorkspaceResource } from "testHelpers/entities";
import { withProxyProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentRowPreview } from "./AgentRowPreview";
import { ResourceCard } from "./ResourceCard";

const meta: Meta<typeof ResourceCard> = {
	title: "modules/resources/ResourceCard",
	component: ResourceCard,
	decorators: [withProxyProvider()],
	args: {
		resource: MockWorkspaceResource,
		agentRow: (agent) => <AgentRowPreview agent={agent} key={agent.id} />,
	},
};

export default meta;
type Story = StoryObj<typeof ResourceCard>;

export const Example: Story = {};

export const BunchOfMetadata: Story = {
	args: {
		resource: {
			...MockWorkspaceResource,
			metadata: [
				{
					key: "CPU(limits, requests)",
					value: "2 cores, 500m",
					sensitive: false,
				},
				{
					key: "container image pull policy",
					value: "Always",
					sensitive: false,
				},
				{ key: "Disk", value: "10GiB", sensitive: false },
				{
					key: "image",
					value: "docker.io/markmilligan/pycharm-community:latest",
					sensitive: false,
				},
				{ key: "kubernetes namespace", value: "oss", sensitive: false },
				{
					key: "memory(limits, requests)",
					value: "4GB, 500mi",
					sensitive: false,
				},
				{
					key: "security context - container",
					value: "run_as_user 1000",
					sensitive: false,
				},
				{
					key: "security context - pod",
					value: "run_as_user 1000 fs_group 1000",
					sensitive: false,
				},
				{ key: "volume", value: "/home/coder", sensitive: false },
				{
					key: "secret",
					value: "3XqfNW0b1bvsGsqud8O6OW6VabH3fwzI",
					sensitive: true,
				},
			],
		},
	},
};
