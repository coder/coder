import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { DateRange } from "react-day-picker";
import { Calendar } from "./Calendar";

const meta: Meta<typeof Calendar> = {
	title: "components/Calendar",
	component: Calendar,
	decorators: [
		(Story) => (
			<div className="rounded-lg border border-solid border-border-default w-fit">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof Calendar>;

export const Single: Story = {
	args: {
		mode: "single",
		selected: new Date("2025-03-15"),
	},
};

export const Range: Story = {
	render: function RangeStory() {
		const [range, setRange] = useState<DateRange>({
			from: new Date("2025-03-10"),
			to: new Date("2025-03-18"),
		});
		return (
			<Calendar
				mode="range"
				selected={range}
				onSelect={(r) => r && setRange(r)}
				numberOfMonths={2}
			/>
		);
	},
};

export const TwoMonths: Story = {
	args: {
		mode: "single",
		numberOfMonths: 2,
		selected: new Date("2025-03-15"),
	},
};

export const DisabledFutureDates: Story = {
	args: {
		mode: "single",
		selected: new Date("2025-03-15"),
		disabled: { after: new Date("2025-03-20") },
	},
};
