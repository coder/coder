import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TemplateCard } from "./TemplateCard";

const meta: Meta<typeof TemplateCard> = {
	title: "pages/TemplateBuilder/TemplateCard",
	component: TemplateCard,
	args: {
		onSelect: fn(),
		detailsUrl: "https://registry.coder.com/templates/docker",
	},
};

export default meta;
type Story = StoryObj<typeof TemplateCard>;

export const Default: Story = {
	args: {
		name: "Docker Containers",
		description: "Provision Docker containers as Coder workspaces.",
		iconUrl: "/icon/docker.svg",
		selected: false,
	},
};

export const Selected: Story = {
	args: {
		name: "Docker Containers",
		description: "Provision Docker containers as Coder workspaces.",
		iconUrl: "/icon/docker.svg",
		selected: true,
	},
};

export const NoIcon: Story = {
	args: {
		name: "Custom Template",
		description: "A template without an icon.",
		selected: false,
	},
};

export const Community: Story = {
	args: {
		name: "Docker Containers",
		description: "Provision Docker containers as Coder workspaces.",
		iconUrl: "/icon/docker.svg",
		official: false,
		selected: false,
	},
};

export const CommunitySelected: Story = {
	args: {
		name: "Docker Containers",
		description: "Provision Docker containers as Coder workspaces.",
		iconUrl: "/icon/docker.svg",
		official: false,
		selected: true,
	},
};

export const LongDescription: Story = {
	args: {
		name: "Kubernetes Pods",
		description:
			"Provision Kubernetes pods as Coder workspaces with full cluster access and custom resource limits.",
		iconUrl: "/icon/k8s.svg",
		selected: false,
	},
};
