import type { Meta, StoryObj } from "@storybook/react";
import { Button } from "components/Button/Button";
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
			<>
				<TooltipProvider>
					<Tooltip open>
						<TooltipTrigger asChild>
							<Button variant="outline">Hover</Button>
						</TooltipTrigger>
						<TooltipContent>Add to library</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof Tooltip>;

export const Default: Story = {};
