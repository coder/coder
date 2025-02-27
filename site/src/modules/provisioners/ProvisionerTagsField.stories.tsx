import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, within } from "@storybook/test";
import type { ProvisionerDaemon } from "api/typesGenerated";
import { type FC, useState } from "react";
import { ProvisionerTagsField } from "./ProvisionerTagsField";

const meta: Meta<typeof ProvisionerTagsField> = {
	title: "modules/provisioners/ProvisionerTagsField",
	component: ProvisionerTagsField,
	args: {
		value: {},
	},
};

export default meta;
type Story = StoryObj<typeof ProvisionerTagsField>;

export const Empty: Story = {
	args: {
		value: {},
	},
};

export const WithInitialValue: Story = {
	args: {
		value: {
			cluster: "dogfood-2",
			env: "gke",
			scope: "organization",
		},
	},
};

type StatefulProvisionerTagsFieldProps = {
	initialValue?: ProvisionerDaemon["tags"];
};

const StatefulProvisionerTagsField: FC<StatefulProvisionerTagsFieldProps> = ({
	initialValue = {},
}) => {
	const [value, setValue] = useState<ProvisionerDaemon["tags"]>(initialValue);
	return <ProvisionerTagsField value={value} onChange={setValue} />;
};

export const OnOverwriteOwner: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const keyInput = canvas.getByLabelText("Tag key");
		const valueInput = canvas.getByLabelText("Tag value");
		const addButton = canvas.getByRole("button", { name: "Add tag" });

		await user.type(keyInput, "owner");
		await user.type(valueInput, "dogfood-2");
		await user.click(addButton);

		await canvas.findByText("Cannot override owner tag");
	},
};

export const OnInvalidScope: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const keyInput = canvas.getByLabelText("Tag key");
		const valueInput = canvas.getByLabelText("Tag value");
		const addButton = canvas.getByRole("button", { name: "Add tag" });

		await user.type(keyInput, "scope");
		await user.type(valueInput, "invalid");
		await user.click(addButton);

		await canvas.findByText("Scope value must be 'organization' or 'user'");
	},
};

export const OnAddTag: Story = {
	render: () => <StatefulProvisionerTagsField />,
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const keyInput = canvas.getByLabelText("Tag key");
		const valueInput = canvas.getByLabelText("Tag value");
		const addButton = canvas.getByRole("button", { name: "Add tag" });

		await user.type(keyInput, "cluster");
		await user.type(valueInput, "dogfood-2");
		await user.click(addButton);

		const addedTag = await canvas.findByTestId("tag-cluster");
		await expect(addedTag).toHaveTextContent("cluster dogfood-2");
	},
};

export const OnRemoveTag: Story = {
	render: () => (
		<StatefulProvisionerTagsField initialValue={{ cluster: "dogfood-2" }} />
	),
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const removeButton = canvas.getByRole("button", { name: "Delete cluster" });

		await user.click(removeButton);

		await expect(canvas.queryByTestId("tag-cluster")).toBeNull();
	},
};
