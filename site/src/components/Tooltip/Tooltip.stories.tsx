import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "./Tooltip";

const meta: Meta<typeof TooltipProvider> = {
	title: "components/Tooltip",
	component: TooltipProvider,
	args: {
		children: (
			<Tooltip open>
				<TooltipTrigger asChild>
					<Button variant="outline">Hover</Button>
				</TooltipTrigger>
				<TooltipContent>Add to library</TooltipContent>
			</Tooltip>
		),
	},
};

export default meta;
type Story = StoryObj<typeof Tooltip>;

export const Default: Story = {};

export const NarrowViewport: Story = {
	args: {
		children: (
			<Tooltip open>
				<TooltipTrigger asChild>
					<Button variant="outline">Hover</Button>
				</TooltipTrigger>
				<TooltipContent>
					Status of your Coder deployment. Only visible for admins! This text is
					long enough to wrap instead of running past the viewport edge.
				</TooltipContent>
			</Tooltip>
		),
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
};
