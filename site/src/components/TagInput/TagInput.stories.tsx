import type { Meta, StoryObj } from "@storybook/react";
import { TagInput } from "./TagInput";

const meta: Meta<typeof TagInput> = {
	title: "components/TagInput",
	component: TagInput,
	decorators: [(Story) => <div style={{ maxWidth: "500px" }}>{Story()}</div>],
};

export default meta;
type Story = StoryObj<typeof TagInput>;

export const Default: Story = {
	args: {
		values: [],
	},
};

export const WithEmptyTags: Story = {
	args: {
		values: ["", "", ""],
	},
};

export const WithLongTags: Story = {
	args: {
		values: [
			"this-is-a-very-long-long-long-tag-that-might-wrap",
			"another-long-tag-example",
			"short",
		],
	},
};

export const WithManyTags: Story = {
	args: {
		values: [
			"tag1",
			"tag2",
			"tag3",
			"tag4",
			"tag5",
			"tag6",
			"tag7",
			"tag8",
			"tag9",
			"tag10",
			"tag11",
			"tag12",
			"tag13",
			"tag14",
			"tag15",
			"tag16",
			"tag17",
			"tag18",
			"tag19",
			"tag20",
		],
	},
};
