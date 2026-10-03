import type { Meta, StoryObj } from "@storybook/react-vite";
import { TemplateConfiguration } from "./TemplateConfiguration";

const meta: Meta<typeof TemplateConfiguration> = {
	title: "pages/TemplateBuilder/TemplateConfiguration",
	component: TemplateConfiguration,
};

export default meta;
type Story = StoryObj<typeof TemplateConfiguration>;

export const Default: Story = {
	args: {
		name: "Docker Containers",
		description: "Provision Docker containers as Coder workspaces.",
		iconUrl: "/icon/docker.svg",
		detailsUrl: "https://registry.coder.com/templates/docker",
		fields: [
			{
				type: "select",
				id: "docker-image",
				label: "Select image",
				placeholder: "Ubuntu (default)",
				options: [
					{ value: "ubuntu", label: "Ubuntu" },
					{ value: "debian", label: "Debian" },
					{ value: "alpine", label: "Alpine" },
					{ value: "fedora", label: "Fedora" },
				],
			},
		],
	},
};

export const NoConfiguration: Story = {
	args: {
		name: "Kubernetes Pods",
		description: "Provision Kubernetes pods as Coder workspaces.",
		iconUrl: "/icon/k8s.svg",
		detailsUrl: "https://registry.coder.com/templates/kubernetes",
	},
};

export const WithoutDetailsLink: Story = {
	args: {
		name: "Custom Template",
		description: "A template without an external details link.",
		iconUrl: "/icon/code.svg",
		fields: [
			{
				type: "select",
				id: "custom-config",
				label: "Configuration",
				placeholder: "Select option",
				options: [
					{ value: "a", label: "Option A" },
					{ value: "b", label: "Option B" },
				],
			},
		],
	},
};

export const WithoutIcon: Story = {
	args: {
		name: "Unnamed Template",
		description: "A template without an icon.",
		detailsUrl: "https://registry.coder.com",
	},
};
