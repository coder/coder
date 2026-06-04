import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { AnimatedNumber } from "./AnimatedNumber";

const meta: Meta<typeof AnimatedNumber> = {
	title: "components/AnimatedNumber",
	component: AnimatedNumber,
};

export default meta;
type Story = StoryObj<typeof AnimatedNumber>;

export const Default: Story = {
	args: {
		value: 75,
		className: "text-2xl font-semibold text-content-primary",
	},
};

export const Percentage: Story = {
	render: function PercentageStory() {
		const [value, setValue] = useState(42);

		return (
			<div className="flex items-center gap-4">
				<span className="text-2xl font-semibold text-content-primary">
					<AnimatedNumber value={`${value}%`} />
				</span>
				<div className="flex gap-2">
					<button
						type="button"
						className="rounded border border-solid border-border px-3 py-1 text-sm bg-surface-primary text-content-primary"
						onClick={() => setValue(Math.floor(Math.random() * 100))}
					>
						Randomize
					</button>
				</div>
			</div>
		);
	},
};

export const Counter: Story = {
	render: function CounterStory() {
		const [count, setCount] = useState(0);

		return (
			<div className="flex items-center gap-4">
				<span className="text-3xl font-semibold tabular-nums text-content-primary">
					<AnimatedNumber value={count.toLocaleString("en-US")} />
				</span>
				<div className="flex gap-2">
					<button
						type="button"
						className="rounded border border-solid border-border px-3 py-1 text-sm bg-surface-primary text-content-primary"
						onClick={() => setCount((c) => c + 1)}
					>
						+1
					</button>
					<button
						type="button"
						className="rounded border border-solid border-border px-3 py-1 text-sm bg-surface-primary text-content-primary"
						onClick={() => setCount((c) => c + 100)}
					>
						+100
					</button>
					<button
						type="button"
						className="rounded border border-solid border-border px-3 py-1 text-sm bg-surface-primary text-content-primary"
						onClick={() => setCount(0)}
					>
						Reset
					</button>
				</div>
			</div>
		);
	},
};
