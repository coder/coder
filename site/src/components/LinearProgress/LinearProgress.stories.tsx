import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import LinearProgress from "./LinearProgress";

const meta: Meta<typeof LinearProgress> = {
	title: "Components/LinearProgress",
	component: LinearProgress,
	args: {
		variant: "determinate",
		value: 40,
	},
	argTypes: {
		variant: {
			control: "inline-radio",
			options: ["determinate", "indeterminate"],
		},
		value: {
			control: { type: "range", min: 0, max: 100, step: 1 },
			if: { arg: "variant", eq: "determinate" },
		},
	},
};

export default meta;
type Story = StoryObj<typeof LinearProgress>;

export const Default: Story = {};

export const Indeterminate: Story = {
	args: {
		variant: "indeterminate",
		value: 0,
	},
	parameters: {
		chromatic: { disable: true },
	},
};

export const Determinate: Story = {
	args: {
		variant: "determinate",
		value: 62,
	},
};

export const DeterminateSamples: Story = {
	render: () => (
		<div className="flex w-full max-w-md flex-col gap-4">
			{([0, 25, 50, 75, 100] as const).map((value) => (
				<div key={value} className="flex flex-col gap-1">
					<span className="text-content-secondary text-xs">{value}%</span>
					<LinearProgress variant="determinate" value={value} />
				</div>
			))}
		</div>
	),
};

export const ControlledDeterminate: Story = {
	render: function ControlledDeterminateRender() {
		const [value, setValue] = useState(0);
		useEffect(() => {
			const id = window.setInterval(() => {
				setValue((previous) => (previous >= 100 ? 0 : previous + 2));
			}, 120);
			return () => window.clearInterval(id);
		}, []);
		return (
			<div className="flex w-full max-w-md flex-col gap-2">
				<span className="text-content-secondary text-xs tabular-nums">
					{value}%
				</span>
				<LinearProgress variant="determinate" value={value} />
			</div>
		);
	},
	parameters: {
		chromatic: { disable: true },
	},
};
