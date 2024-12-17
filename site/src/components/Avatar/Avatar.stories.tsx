import type { Meta, StoryObj } from "@storybook/react";
import { Avatar, AvatarFallback, AvatarImage } from "./Avatar";

const meta: Meta<typeof Avatar> = {
	title: "components/Avatar",
	component: Avatar,
	args: {
		children: <AvatarImage src="https://github.com/kylecarbs.png" />,
	},
};

export default meta;
type Story = StoryObj<typeof Avatar>;

export const ImageLgSize: Story = {
	args: { size: "lg" },
};

export const ImageDefaultSize: Story = {};

export const ImageSmSize: Story = {
	args: { size: "sm" },
};

export const IconLgSize: Story = {
	args: {
		size: "lg",
		variant: "icon",
		children: (
			<AvatarImage src="https://em-content.zobj.net/source/apple/391/billed-cap_1f9e2.png" />
		),
	},
};

export const IconDefaultSize: Story = {
	args: {
		variant: "icon",
		children: (
			<AvatarImage src="https://em-content.zobj.net/source/apple/391/billed-cap_1f9e2.png" />
		),
	},
};

export const IconSmSize: Story = {
	args: {
		variant: "icon",
		size: "sm",
		children: (
			<AvatarImage src="https://em-content.zobj.net/source/apple/391/billed-cap_1f9e2.png" />
		),
	},
};

export const FallbackLgSize: Story = {
	args: {
		size: "lg",

		children: <AvatarFallback>AR</AvatarFallback>,
	},
};

export const FallbackDefaultSize: Story = {
	args: {
		children: <AvatarFallback>AR</AvatarFallback>,
	},
};

export const FallbackSmSize: Story = {
	args: {
		size: "sm",
		children: <AvatarFallback>AR</AvatarFallback>,
	},
};
