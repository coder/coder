import { MockTemplate, MockTemplateVersion } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { useState } from "react";
import { spyOn, userEvent, within } from "storybook/test";
import { daysAgo } from "utils/time";
import { TemplateVersionSelect } from "./TemplateVersionSelect";

const meta: Meta<typeof TemplateVersionSelect> = {
	title: "modules/tasks/TaskPrompt/TemplateVersionSelect",
	component: TemplateVersionSelect,
	args: {
		activeVersionId: MockTemplateVersion.id,
		templateId: MockTemplate.id,
		value: MockTemplateVersion.id,
	},
	render: ({ value: defaultValue, ...args }) => {
		const [value, setValue] = useState(defaultValue);
		return (
			<TemplateVersionSelect {...args} value={value} onValueChange={setValue} />
		);
	},
};

export default meta;
type Story = StoryObj<typeof TemplateVersionSelect>;

const MockVersions = [
	{
		...MockTemplateVersion,
		id: "v1.0.0",
		name: "v1.0.0",
		created_at: daysAgo(3),
	},
	{
		...MockTemplateVersion,
		id: "v2.0.0",
		name: "v2.0.0",
		created_at: daysAgo(2),
	},
	{
		...MockTemplateVersion,
		id: "v3.0.0",
		name: "v3.0.0",
		created_at: daysAgo(1),
	},
];

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplateVersions").mockImplementation(() => {
			return new Promise(() => {});
		});
	},
};

export const Loaded: Story = {
	args: {
		activeVersionId: MockVersions[2]!.id,
		value: MockVersions[2]!.id,
	},
	beforeEach: () => {
		spyOn(API, "getTemplateVersions").mockResolvedValue(MockVersions);
	},
};

export const Open: Story = {
	args: {
		activeVersionId: MockVersions[2]!.id,
		value: MockVersions[2]!.id,
	},
	beforeEach: () => {
		spyOn(API, "getTemplateVersions").mockResolvedValue(MockVersions);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = await canvas.findByRole("combobox");
		await userEvent.click(trigger);
	},
};
