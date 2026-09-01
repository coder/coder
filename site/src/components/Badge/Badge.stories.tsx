import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	CheckIcon,
	DatabaseIcon,
	SettingsIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { Spinner } from "#/components/Spinner/Spinner";
import { Badge } from "./Badge";
import { BadgeGroup } from "./PresetBadges";

const meta: Meta<typeof Badge> = {
	title: "components/Badge",
};

export default meta;
type Story = StoryObj<typeof Badge>;

export const Default: Story = {
	render: () => (
		<BadgeGroup>
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
		</BadgeGroup>
	),
};

export const Warning: Story = {
	render: () => (
		<BadgeGroup>
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
		</BadgeGroup>
	),
};

export const Destructive: Story = {
	render: () => (
		<BadgeGroup>
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
		</BadgeGroup>
	),
};

export const Info: Story = {
	render: () => (
		<BadgeGroup>
			<Badge variant="info" size="xs">
				Info
			</Badge>
			<Badge variant="info" size="sm">
				Info
			</Badge>
			<Badge variant="info" size="md">
				Info
			</Badge>
		</BadgeGroup>
	),
};

export const Green: Story = {
	render: () => (
		<BadgeGroup>
			<Badge variant="green" size="xs">
				Green
			</Badge>
			<Badge variant="green" size="sm">
				Green
			</Badge>
			<Badge variant="green" size="md">
				Green
			</Badge>
		</BadgeGroup>
	),
};

export const Purple: Story = {
	render: () => (
		<BadgeGroup>
			<Badge variant="purple" size="xs">
				Purple
			</Badge>
			<Badge variant="purple" size="sm">
				Purple
			</Badge>
			<Badge variant="purple" size="md">
				Purple
			</Badge>
		</BadgeGroup>
	),
};

export const Magenta: Story = {
	render: () => (
		<BadgeGroup>
			<Badge variant="magenta" size="xs">
				Magenta
			</Badge>
			<Badge variant="magenta" size="sm">
				Magenta
			</Badge>
			<Badge variant="magenta" size="md">
				Magenta
			</Badge>
		</BadgeGroup>
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

export const StatusWithIcon: Story = {
	render: () => (
		<BadgeGroup>
			<Badge variant="info" size="md" role="status">
				<Spinner loading />
				Running
			</Badge>
			<Badge variant="green" size="md" role="status">
				<CheckIcon />
				Success
			</Badge>
			<Badge variant="destructive" size="md" role="status">
				<TriangleAlertIcon />
				Failed
			</Badge>
		</BadgeGroup>
	),
};
