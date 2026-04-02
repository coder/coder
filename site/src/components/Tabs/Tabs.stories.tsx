import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	LinkTabs,
	LinkTabsList,
	TabLink,
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "./Tabs";

const meta: Meta<typeof LinkTabs> = {
	title: "components/Tabs",
	component: LinkTabs,
};

export default meta;
type Story = StoryObj<typeof LinkTabs>;

export const LinkNavigation: Story = {
	args: {
		active: "tab-1",
		children: (
			<LinkTabsList>
				<TabLink value="tab-1" to="">
					Tab 1
				</TabLink>
				<TabLink value="tab-2" to="tab-3">
					Tab 2
				</TabLink>
				<TabLink value="tab-3" to="tab-4">
					Tab 3
				</TabLink>
			</LinkTabsList>
		),
	},
	render: (args) => <LinkTabs {...args} />,
};

export const RadixInsideBox: StoryObj = {
	render: () => (
		<Tabs defaultValue="a">
			<TabsList variant="insideBox">
				<TabsTrigger value="a">Alpha</TabsTrigger>
				<TabsTrigger value="b">Beta</TabsTrigger>
			</TabsList>
			<TabsContent value="a" className="p-4">
				Panel A
			</TabsContent>
			<TabsContent value="b" className="p-4">
				Panel B
			</TabsContent>
		</Tabs>
	),
};

export const RadixOutsideBox: StoryObj = {
	render: () => (
		<Tabs defaultValue="a">
			<TabsList variant="outsideBox">
				<TabsTrigger value="a">Alpha</TabsTrigger>
				<TabsTrigger value="b">Beta</TabsTrigger>
			</TabsList>
			<TabsContent value="a" className="p-4">
				Panel A
			</TabsContent>
			<TabsContent value="b" className="p-4">
				Panel B
			</TabsContent>
		</Tabs>
	),
};
