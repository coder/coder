import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	AlphaBadge,
	Badges,
	DeprecatedBadge,
	DisabledBadge,
	EnabledBadge,
	EnterpriseBadge,
	PremiumBadge,
} from "./Badges";

const meta: Meta<typeof Badges> = {
	title: "components/Badges",
	component: Badges,
	args: {},
};

export default meta;
type Story = StoryObj<typeof Badges>;

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
