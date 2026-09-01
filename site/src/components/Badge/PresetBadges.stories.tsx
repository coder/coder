import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	AlphaBadge,
	BadgeGroup,
	DeprecatedBadge,
	DisabledBadge,
	EnabledBadge,
	EnterpriseBadge,
	PremiumBadge,
} from "./PresetBadges";

const meta: Meta<typeof BadgeGroup> = {
	title: "components/Badge/PresetBadges",
	component: BadgeGroup,
};

export default meta;
type Story = StoryObj<typeof BadgeGroup>;

export const Enabled: Story = {
	args: {
		children: <EnabledBadge />,
	},
};

export const Disabled: Story = {
	args: {
		children: <DisabledBadge />,
	},
};

export const Premium: Story = {
	args: {
		children: <PremiumBadge />,
	},
};

export const Alpha: Story = {
	args: {
		children: <AlphaBadge />,
	},
};

export const Enterprise: Story = {
	args: {
		children: <EnterpriseBadge />,
	},
};

export const Deprecated: Story = {
	args: {
		children: <DeprecatedBadge />,
	},
};
