import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Switch } from "#/components/Switch/Switch";
import { ModuleConfiguration } from "./ModuleConfiguration";

const meta: Meta<typeof ModuleConfiguration> = {
	title: "pages/TemplateBuilder/ModuleConfiguration",
	component: ModuleConfiguration,
	args: {
		onRemove: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ModuleConfiguration>;

const TextInputField: React.FC<{
	id: string;
	label: string;
	required?: boolean;
	placeholder?: string;
}> = ({ id, label, required, placeholder }) => (
	<div className="flex flex-col gap-1 w-full">
		<Label htmlFor={id} className="text-sm font-normal text-content-primary">
			{label}
			{required && " *"}
		</Label>
		<Input id={id} placeholder={placeholder} />
	</div>
);

const RadioField: React.FC<{
	label: string;
	required?: boolean;
	options: { value: string; label: string; iconUrl?: string }[];
}> = ({ label, required, options }) => (
	<div className="flex flex-col gap-1 w-full">
		<Label className="text-sm font-normal text-content-primary">
			{label}
			{required && " *"}
		</Label>
		<RadioGroup className="gap-2">
			{options.map((opt) => (
				<div key={opt.value} className="flex items-start gap-2">
					<div className="flex items-center py-1">
						<RadioGroupItem id={`radio-${opt.value}`} value={opt.value} />
					</div>
					<Label
						htmlFor={`radio-${opt.value}`}
						className="flex items-center gap-1 text-sm font-normal text-content-primary leading-6"
					>
						{opt.iconUrl && (
							<img src={opt.iconUrl} alt="" className="size-6 object-contain" />
						)}
						{opt.label}
					</Label>
				</div>
			))}
		</RadioGroup>
	</div>
);

const SelectField: React.FC<{
	id: string;
	label: string;
	optional?: boolean;
	placeholder?: string;
	options: { value: string; label: string }[];
}> = ({ id, label, optional, placeholder, options }) => (
	<div className="flex flex-col gap-1 w-full">
		<Label htmlFor={id} className="text-sm font-normal text-content-primary">
			{label}
			{optional && <span className="text-content-secondary"> (optional)</span>}
		</Label>
		<Select>
			<SelectTrigger id={id}>
				<SelectValue placeholder={placeholder ?? "Select..."} />
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

const SwitchField: React.FC<{
	id: string;
	label: string;
	defaultChecked?: boolean;
}> = ({ id, label, defaultChecked }) => (
	<div className="flex items-start gap-2">
		<div className="p-0.5">
			<Switch id={id} defaultChecked={defaultChecked} />
		</div>
		<Label
			htmlFor={id}
			className="text-sm font-normal text-content-primary leading-6"
		>
			{label}
		</Label>
	</div>
);

const SwitchGroupField: React.FC<{
	label: string;
	required?: boolean;
	switches: { id: string; label: string; defaultChecked?: boolean }[];
}> = ({ label, required, switches }) => (
	<div className="flex flex-col gap-1 w-full">
		<Label className="text-sm font-normal text-content-primary">
			{label}
			{required && " *"}
		</Label>
		<div className="flex flex-col gap-2">
			{switches.map((s) => (
				<SwitchField
					key={s.id}
					id={s.id}
					label={s.label}
					defaultChecked={s.defaultChecked}
				/>
			))}
		</div>
	</div>
);

export const Default: Story = {
	args: {
		name: "Claude Code",
		description: "Run the Claude Code agent in your workspace.",
		iconUrl: "/icon/claude.svg",
		detailsUrl: "https://registry.coder.com/modules/claude-code",
	},
	render: (args) => (
		<ModuleConfiguration {...args}>
			<div className="flex flex-col gap-6">
				<TextInputField
					id="anthropic-api-key"
					label="Anthropic API key"
					required
					placeholder="Enter API key"
				/>
				<RadioField
					label="Other example"
					required
					options={[
						{ value: "opt-1", label: "Radio text", iconUrl: "/icon/aws.svg" },
						{ value: "opt-2", label: "Radio text", iconUrl: "/icon/aws.svg" },
						{ value: "opt-3", label: "Radio text", iconUrl: "/icon/aws.svg" },
						{ value: "opt-4", label: "Radio text", iconUrl: "/icon/aws.svg" },
					]}
				/>
			</div>
			<div className="flex flex-col gap-6">
				<SelectField
					id="one-more-example"
					label="One more example"
					optional
					options={[
						{ value: "a", label: "Option A" },
						{ value: "b", label: "Option B" },
						{ value: "c", label: "Option C" },
					]}
				/>
				<SwitchGroupField
					label="Other example"
					required
					switches={[
						{ id: "switch-1", label: "Explaining text", defaultChecked: true },
						{ id: "switch-2", label: "Explaining text", defaultChecked: false },
					]}
				/>
			</div>
		</ModuleConfiguration>
	),
};

export const NoConfiguration: Story = {
	args: {
		name: "Git Clone",
		description: "Clone a Git repository into your workspace on start.",
		iconUrl: "/icon/git.svg",
		detailsUrl: "https://registry.coder.com/modules/git-clone",
	},
};

export const WithoutDetailsLink: Story = {
	args: {
		name: "Custom Module",
		description: "A module without an external details link.",
		iconUrl: "/icon/code.svg",
	},
	render: (args) => (
		<ModuleConfiguration {...args}>
			<TextInputField
				id="custom-input"
				label="Configuration value"
				required
				placeholder="Enter value"
			/>
		</ModuleConfiguration>
	),
};

export const WithoutIcon: Story = {
	args: {
		name: "Unnamed Module",
		description: "A module without an icon.",
		detailsUrl: "https://registry.coder.com",
	},
	render: (args) => (
		<ModuleConfiguration {...args}>
			<SwitchField id="enabled" label="Enabled" defaultChecked />
		</ModuleConfiguration>
	),
};
