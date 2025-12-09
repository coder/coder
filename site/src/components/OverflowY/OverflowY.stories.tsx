import type { Meta, StoryObj } from "@storybook/react-vite";
import { OverflowY } from "./OverflowY";

const numbers: number[] = [];
for (let i = 0; i < 20; i++) {
	numbers.push(i + 1);
}

const meta: Meta<typeof OverflowY> = {
	title: "components/OverflowY",
	component: OverflowY,
	args: {
		maxHeight: 400,
		children: numbers.map((num, i) => (
			<p
				key={num}
				css={{
					backgroundColor: i % 2 === 0 ? "white" : "gray",
				}}
				className="h-[50px] p-0 m-0 text-black"
			>
				Element {num}
			</p>
		)),
	},
};

export default meta;

type Story = StoryObj<typeof OverflowY>;
const Example: Story = {};

export { Example as OverflowY };
