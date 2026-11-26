import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent } from "storybook/test";
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

export const TallerThanViewport: Story = {
	args: {
		children: (
			<Tooltip open>
				<TooltipTrigger asChild>
					<Button variant="outline">Hover</Button>
				</TooltipTrigger>
				<TooltipContent>
					{Array.from({ length: 60 }, (_, index) => (
						<p key={index} className="m-0">
							Line {index + 1} of a tooltip taller than the viewport.
						</p>
					))}
				</TooltipContent>
			</Tooltip>
		),
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
		// A plain tooltip cannot take focus (ARIA tooltip pattern), so its
		// scroll region is not keyboard focusable.
		a11y: {
			config: {
				rules: [{ id: "scrollable-region-focusable", enabled: false }],
			},
		},
	},
};

export const InteractiveTallerThanViewport: Story = {
	args: {
		children: (
			<Tooltip interactive>
				<TooltipTrigger>Details</TooltipTrigger>
				<TooltipContent>
					{Array.from({ length: 60 }, (_, index) => (
						<p key={index} className="m-0">
							Line {index + 1} of a tooltip taller than the viewport.
						</p>
					))}
				</TooltipContent>
			</Tooltip>
		),
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
	play: async () => {
		await userEvent.tab();
		await userEvent.tab();
	},
};
