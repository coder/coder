import type { Meta, StoryObj } from "@storybook/react";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "./SettingsHeader";
import { docs } from "utils/docs";

const meta: Meta<typeof SettingsHeader> = {
	title: "components/SettingsHeader",
	component: SettingsHeader,
};

export default meta;
type Story = StoryObj<typeof SettingsHeader>;

export const PrimaryHeaderOnly: Story = {
	args: {
		children: <SettingsHeaderTitle>This is a header</SettingsHeaderTitle>,
	},
};

export const PrimaryHeaderWithDescription: Story = {
	args: {
		children: (
			<>
				<SettingsHeaderTitle>Another primary header</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					This description can be any ReactNode. This provides more options for
					composition.
				</SettingsHeaderDescription>
			</>
		),
	},
};

export const PrimaryHeaderWithDescriptionAndDocsLink: Story = {
	args: {
		children: (
			<>
				<SettingsHeaderTitle>Another primary header</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					This description can be any ReactNode. This provides more options for
					composition.
				</SettingsHeaderDescription>
			</>
		),
		actions: <SettingsHeaderDocsLink href={docs("/admin/external-auth")} />,
	},
};

export const SecondaryHeaderWithDescription: Story = {
	args: {
		children: (
			<>
				<SettingsHeaderTitle level="h6" hierarchy="secondary">
					This is a secondary header.
				</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					The header's styling is completely independent of its semantics. Both
					can be adjusted independently to help avoid invalid HTML.
				</SettingsHeaderDescription>
			</>
		),
	},
};

export const SecondaryHeaderWithDescriptionAndDocsLink: Story = {
	args: {
		children: (
			<>
				<SettingsHeaderTitle level="h3" hierarchy="secondary">
					Another secondary header
				</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Nothing to add, really.
				</SettingsHeaderDescription>
			</>
		),
		actions: <SettingsHeaderDocsLink href={docs("/admin/external-auth")} />,
	},
};
