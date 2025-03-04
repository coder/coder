import type { Meta, StoryObj } from "@storybook/react";
import { expect, fn, userEvent, within } from "@storybook/test";
import { useState } from "react";
import { chromatic } from "testHelpers/chromatic";
import { MockTemplateVersion } from "testHelpers/entities";
import { ProvisionerTagsPopover } from "./ProvisionerTagsPopover";

const meta: Meta<typeof ProvisionerTagsPopover> = {
	title: "pages/TemplateVersionEditorPage/ProvisionerTagsPopover",
	parameters: {
		chromatic,
		layout: "centered",
	},
	component: ProvisionerTagsPopover,
	args: {
		tags: MockTemplateVersion.job.tags,
	},
};

export default meta;
type Story = StoryObj<typeof ProvisionerTagsPopover>;

export const Closed: Story = {};

export const Open: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
	},
};

export const OnTagsChange: Story = {
	parameters: {
		chromatic: { disableSnapshot: true },
	},
	args: {
		tags: {},
	},
	render: (args) => {
		const [tags, setTags] = useState(args.tags);
		return <ProvisionerTagsPopover tags={tags} onTagsChange={fn(setTags)} />;
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		const expandButton = canvas.getByRole("button", {
			name: "Expand provisioner tags",
		});
		await userEvent.click(expandButton);

		const keyInput = await canvas.findByLabelText("Tag key");
		const valueInput = await canvas.findByLabelText("Tag value");
		const addButton = await canvas.findByRole("button", {
			name: "Add tag",
			hidden: true,
		});

		await user.type(keyInput, "cluster");
		await user.type(valueInput, "dogfood-2");
		await user.click(addButton);
		const addedTag = await canvas.findByTestId("tag-cluster");
		await expect(addedTag).toHaveTextContent("cluster dogfood-2");

		const removeButton = canvas.getByRole("button", {
			name: "Delete cluster",
			hidden: true,
		});
		await user.click(removeButton);
		await expect(canvas.queryByTestId("tag-cluster")).toBeNull();
	},
};
