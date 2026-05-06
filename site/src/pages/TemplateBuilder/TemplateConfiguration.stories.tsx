import type { Meta, StoryObj } from "@storybook/react-vite";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { TemplateConfiguration } from "./TemplateConfiguration";

const meta: Meta<typeof TemplateConfiguration> = {
	title: "pages/TemplateBuilder/TemplateConfiguration",
	component: TemplateConfiguration,
};

export default meta;
type Story = StoryObj<typeof TemplateConfiguration>;

const SelectField: React.FC<{
	id: string;
	label: string;
	placeholder: string;
	options: { value: string; label: string }[];
}> = ({ id, label, placeholder, options }) => (
	<div className="flex flex-col gap-1 w-[380px] max-w-full">
		<Label htmlFor={id} className="text-sm font-normal text-content-primary">
			{label}
		</Label>
		<Select>
			<SelectTrigger id={id}>
				<SelectValue placeholder={placeholder} />
			</SelectTrigger>
			<SelectContent>
				{options.map((opt) => (
					<SelectItem key={opt.value} value={opt.value}>
						{opt.label}
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	</div>
);

export const Default: Story = {
	args: {
		name: "Docker Containers",
		description: "Provision Docker containers as Coder workspaces.",
		iconUrl: "/icon/docker.svg",
		detailsUrl: "https://registry.coder.com/templates/docker",
	},
	render: (args) => (
		<TemplateConfiguration {...args}>
			<SelectField
				id="docker-image"
				label="Select image"
				placeholder="Ubuntu (default)"
				options={[
					{ value: "ubuntu", label: "Ubuntu" },
					{ value: "debian", label: "Debian" },
					{ value: "alpine", label: "Alpine" },
					{ value: "fedora", label: "Fedora" },
				]}
			/>
		</TemplateConfiguration>
	),
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
	},
	render: (args) => (
		<TemplateConfiguration {...args}>
			<SelectField
				id="custom-config"
				label="Configuration"
				placeholder="Select option"
				options={[
					{ value: "a", label: "Option A" },
					{ value: "b", label: "Option B" },
				]}
			/>
		</TemplateConfiguration>
	),
};

export const WithoutIcon: Story = {
	args: {
		name: "Unnamed Template",
		description: "A template without an icon.",
		detailsUrl: "https://registry.coder.com",
	},
};
