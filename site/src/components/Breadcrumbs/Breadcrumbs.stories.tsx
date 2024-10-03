import type { Meta, StoryObj } from "@storybook/react";
import { Breadcrumbs, Crumb } from "./Breadcrumbs";

const meta: Meta<typeof Breadcrumbs> = {
	title: "components/Breadcrumbs",
	component: Breadcrumbs,
};

type Story = StoryObj<typeof Breadcrumbs>;
export default meta;

export const Example: Story = {
	args: {
		children: (
			<>
				<Crumb href="/wibble">Wibble</Crumb>
				<Crumb href="/wibble/wobble">Wobble</Crumb>
				<Crumb href="/wibble/wobble/wooble">Wooble</Crumb>
			</>
		),
	},
};

export const IdpSettings: Story = {
	args: {
		children: (
			<>
				<Crumb>Organizations</Crumb>
				<Crumb href="/organizations/wobble">Wobble</Crumb>
				<Crumb href="/organizations/wobble/groups">Groups</Crumb>
				<Crumb active>IdP Sync</Crumb>
			</>
		),
	},
};

export const GroupSettings: Story = {
	args: {
		children: (
			<>
				<Crumb>Organizations</Crumb>
				<Crumb href="/organizations/wobble">Wobble</Crumb>
				<Crumb href="/organizations/wobble/groups">Groups</Crumb>
				<Crumb href="/organizations/wobble/groups/wibble">Wibble</Crumb>
				<Crumb href="/organizations/wobble/groups/wibble/settings" active>
					Group settings
				</Crumb>
			</>
		),
	},
};
