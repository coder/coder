import type { Meta, StoryObj } from "@storybook/react";
import { RadioGroup, RadioGroupItem } from "./RadioGroup";

const meta: Meta<typeof RadioGroup> = {
	title: "components/RadioGroup",
	component: RadioGroup,
	args: {},
};

export default meta;
type Story = StoryObj<typeof RadioGroup>;

export const Default: Story = {
	render: () => (
		<RadioGroup defaultValue="option-1">
			<div className="flex items-center space-x-2">
				<RadioGroupItem value="option-1" id="option-1" tabIndex={0} />
				<label htmlFor="option-1" className="text-content-primary text-sm">
					Option 1
				</label>
			</div>
			<div className="flex items-center space-x-2">
				<RadioGroupItem value="option-2" id="option-2" tabIndex={0} />
				<label htmlFor="option-2" className="text-content-primary text-sm">
					Option 2
				</label>
			</div>
			<div className="flex items-center space-x-2">
				<RadioGroupItem value="option-3" id="option-3" tabIndex={0} />
				<label htmlFor="option-3" className="text-content-primary text-sm">
					Option 3
				</label>
			</div>
		</RadioGroup>
	),
};

export const WithDisabledOptions: Story = {
	render: () => (
		<RadioGroup defaultValue="option-1">
			<div className="flex items-center space-x-2">
				<RadioGroupItem value="option-1" id="disabled-1" disabled />
				<label htmlFor="disabled-1" className="text-content-disabled text-sm">
					Disabled Selected Option
				</label>
			</div>
			<div className="flex items-center space-x-2">
				<RadioGroupItem value="option-2" id="disabled-2" disabled />
				<label htmlFor="disabled-2" className="text-content-disabled text-sm">
					Disabled Not Selected Option
				</label>
			</div>
		</RadioGroup>
	),
};
