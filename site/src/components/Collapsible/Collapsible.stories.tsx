import type { Meta, StoryObj } from "@storybook/react";
import { Button } from "components/Button/Button";
import { ChevronsUpDown } from "lucide-react";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "./Collapsible";

const meta: Meta<typeof Collapsible> = {
	title: "components/Collapsible",
	component: Collapsible,
	args: {
		className: "w-[350px] space-y-2",
		children: (
			<>
				<div className="flex items-center justify-between space-x-4 px-4">
					<h4 className="text-sm font-semibold">
						@peduarte starred 3 repositories
					</h4>
					<CollapsibleTrigger asChild>
						<Button size="sm">
							<ChevronsUpDown className="h-4 w-4" />
							<span className="sr-only">Toggle</span>
						</Button>
					</CollapsibleTrigger>
				</div>
				<div className="rounded-md border px-4 py-2 font-mono text-sm shadow-sm">
					@radix-ui/primitives
				</div>
				<CollapsibleContent className="space-y-2">
					<div className="rounded-md border px-4 py-2 font-mono text-sm shadow-sm">
						@radix-ui/colors
					</div>
					<div className="rounded-md border px-4 py-2 font-mono text-sm shadow-sm">
						@stitches/react
					</div>
				</CollapsibleContent>
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof Collapsible>;

export const Close: Story = {};

export const Open: Story = {
	args: {
		open: true,
	},
};
