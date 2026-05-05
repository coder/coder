import type { Meta, StoryObj } from "@storybook/react-vite";
import { DatabaseIcon, SettingsIcon, TriangleAlertIcon } from "lucide-react";
import { Badges } from "#/components/Badges/Badges";
import { Badge } from "./Badge";

const meta: Meta<typeof Badge> = {
	title: "components/Badge",
};

export default meta;
type Story = StoryObj<typeof Badge>;

export const Default: Story = {
	render: () => (
		<Badges>
			<Badge size="xs">
				<DatabaseIcon />
				Text
			</Badge>
			<Badge size="sm">
				<DatabaseIcon />
				Text
			</Badge>
			<Badge size="md">
				<DatabaseIcon />
				Text
			</Badge>
		</Badges>
	),
};

export const Warning: Story = {
	render: () => (
		<Badges>
			<Badge variant="warning" size="xs">
				Warning
				<TriangleAlertIcon />
			</Badge>
			<Badge variant="warning" size="sm">
				<TriangleAlertIcon />
				Warning
			</Badge>
			<Badge variant="warning" size="md">
				<TriangleAlertIcon />
				Warning
			</Badge>
		</Badges>
	),
};

export const Destructive: Story = {
	render: () => (
		<Badges>
			<Badge variant="destructive" size="xs">
				Destructive
				<TriangleAlertIcon />
			</Badge>
			<Badge variant="destructive" size="sm">
				<TriangleAlertIcon />
				Destructive
			</Badge>
			<Badge variant="destructive" size="md">
				<TriangleAlertIcon />
				Destructive
			</Badge>
		</Badges>
	),
};

export const Info: Story = {
	render: () => (
		<Badges>
			<Badge variant="info" size="xs">
				Info
			</Badge>
			<Badge variant="info" size="sm">
				Info
			</Badge>
			<Badge variant="info" size="md">
				Info
			</Badge>
		</Badges>
	),
};

export const Green: Story = {
	render: () => (
		<Badges>
			<Badge variant="green" size="xs">
				Green
			</Badge>
			<Badge variant="green" size="sm">
				Green
			</Badge>
			<Badge variant="green" size="md">
				Green
			</Badge>
		</Badges>
	),
};

export const Purple: Story = {
	render: () => (
		<Badges>
			<Badge variant="purple" size="xs">
				Purple
			</Badge>
			<Badge variant="purple" size="sm">
				Purple
			</Badge>
			<Badge variant="purple" size="md">
				Purple
			</Badge>
		</Badges>
	),
};

export const Magenta: Story = {
	render: () => (
		<Badges>
			<Badge variant="magenta" size="xs">
				Magenta
			</Badge>
			<Badge variant="magenta" size="sm">
				Magenta
			</Badge>
			<Badge variant="magenta" size="md">
				Magenta
			</Badge>
		</Badges>
	),
};

export const SmallWithIcon: Story = {
	render: () => (
		<Badge variant="default" size="sm">
			<SettingsIcon />
			Preset
		</Badge>
	),
};

export const MediumWithIcon: Story = {
	render: () => (
		<Badge variant="warning" size="md">
			<TriangleAlertIcon />
			Immutable
		</Badge>
	),
};
