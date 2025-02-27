import type { Meta, StoryObj } from "@storybook/react";
import {
	AlphaBadge,
	Badges,
	DisabledBadge,
	EaBadge,
	EnabledBadge,
	EntitledBadge,
	HealthyBadge,
	NotHealthyBadge,
	NotReachableBadge,
	NotRegisteredBadge,
	PremiumBadge,
	PreviewBadge,
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
export const Entitled: Story = {
	args: {
		children: <EntitledBadge />,
	},
};
export const ProxyStatus: Story = {
	args: {
		children: (
			<>
				<HealthyBadge />
				<HealthyBadge derpOnly />
				<NotHealthyBadge />
				<NotRegisteredBadge />
				<NotReachableBadge />
			</>
		),
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
export const Preview: Story = {
	args: {
		children: <PreviewBadge />,
	},
};
export const Alpha: Story = {
	args: {
		children: <AlphaBadge />,
	},
};
export const EarlyAccess: Story = {
	args: {
		children: <EaBadge />,
	},
};
