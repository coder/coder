import type { Meta, StoryObj } from "@storybook/react-vite";
import { SidebarNavItem } from "./Sidebar";
import { SidebarLayout } from "./SidebarLayout";

const meta: Meta<typeof SidebarLayout> = {
	title: "components/Sidebar/SidebarLayout",
	component: SidebarLayout,
	parameters: {
		layout: "fullscreen",
		reactRouter: {
			location: { path: "/example" },
			routing: [
				{
					path: "/",
					useStoryElement: true,
					children: [{ path: "example", element: <>Example page</> }],
				},
			],
		},
	},
	args: {
		sidebar: (
			<div className="flex flex-col gap-1 w-60">
				<SidebarNavItem href="/example">Example</SidebarNavItem>
				<SidebarNavItem href="/other">Other</SidebarNavItem>
			</div>
		),
	},
};

export default meta;
type Story = StoryObj<typeof SidebarLayout>;

export const Default: Story = {};
