import { useTheme } from "@emotion/react";
import type { Meta, StoryObj } from "@storybook/react";
import { useState } from "react";
import { FeatureBadge } from "./FeatureBadge";

const meta: Meta<typeof FeatureBadge> = {
	title: "components/FeatureBadge",
	component: FeatureBadge,
	args: {
		type: "beta",
	},
};

export default meta;
type Story = StoryObj<typeof FeatureBadge>;

export const SmallInteractiveBeta: Story = {
	args: {
		type: "beta",
		size: "sm",
		variant: "interactive",
	},
};

export const SmallInteractiveExperimental: Story = {
	args: {
		type: "experimental",
		size: "sm",
		variant: "interactive",
	},
};

export const LargeInteractiveBeta: Story = {
	args: {
		type: "beta",
		size: "lg",
		variant: "interactive",
	},
};

export const LargeStaticBeta: Story = {
	args: {
		type: "beta",
		size: "lg",
		variant: "static",
	},
};

export const HoverControlledByParent: Story = {
	args: {
		type: "experimental",
		size: "sm",
	},

	decorators: (Story, context) => {
		const theme = useTheme();
		const [isHovering, setIsHovering] = useState(false);

		return (
			<button
				type="button"
				onPointerEnter={() => setIsHovering(true)}
				onPointerLeave={() => setIsHovering(false)}
				css={[
					{
						backgroundColor: theme.palette.background.default,
						color: theme.palette.text.primary,
						display: "flex",
						flexFlow: "row nowrap",
						alignItems: "center",
						columnGap: "16px",
					},
					isHovering && {
						backgroundColor: "green",
					},
				]}
			>
				<span>Blah</span>
				{Story({
					args: {
						...context.initialArgs,
						variant: isHovering ? "staticHover" : "static",
					},
				})}
			</button>
		);
	},
};
