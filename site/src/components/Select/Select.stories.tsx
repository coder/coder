import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectLabel,
	SelectTrigger,
	SelectValue,
} from "./Select";

const meta: Meta<typeof Select> = {
	title: "components/Select",
	component: Select,
	args: {
		children: (
			<>
				<SelectTrigger className="w-[180px]">
					<SelectValue placeholder="Select a fruit" />
				</SelectTrigger>
				<SelectContent>
					<SelectGroup>
						<SelectLabel>Fruits</SelectLabel>
						<SelectItem value="apple">Apple</SelectItem>
						<SelectItem value="banana">Banana</SelectItem>
						<SelectItem value="blueberry">Blueberry</SelectItem>
						<SelectItem value="grapes">Grapes</SelectItem>
						<SelectItem value="pineapple">Pineapple</SelectItem>
					</SelectGroup>
				</SelectContent>
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof Select>;

export const Close: Story = {};

export const Open: Story = {
	args: {
		open: true,
	},
};

export const SelectedClose: Story = {
	args: {
		value: "apple",
	},
};

export const SelectedOpen: Story = {
	args: {
		value: "apple",
		open: true,
	},
};

export const NarrowViewport: Story = {
	args: {
		open: true,
		children: (
			<>
				<SelectTrigger className="w-[180px]">
					<SelectValue placeholder="Select a login type" />
				</SelectTrigger>
				<SelectContent>
					<SelectItem value="service-account" className="whitespace-normal">
						Service account: cannot log in interactively. Intended for automated
						pipelines, bots, and other non-human access.
					</SelectItem>
					{Array.from({ length: 24 }, (_, i) => (
						<SelectItem key={i} value={`option-${i + 1}`}>
							Option {i + 1}
						</SelectItem>
					))}
				</SelectContent>
			</>
		),
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
};
