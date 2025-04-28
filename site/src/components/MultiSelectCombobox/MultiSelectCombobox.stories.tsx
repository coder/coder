import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import { MockOrganization, MockOrganization2 } from "testHelpers/entities";
import { MultiSelectCombobox } from "./MultiSelectCombobox";

const organizations = [MockOrganization, MockOrganization2];

const meta: Meta<typeof MultiSelectCombobox> = {
	title: "components/MultiSelectCombobox",
	component: MultiSelectCombobox,
	args: {
		hidePlaceholderWhenSelected: true,
		placeholder: "Select organization",
		emptyIndicator: (
			<p className="text-center text-md text-content-primary">
				All organizations selected
			</p>
		),
		options: organizations.map((org) => ({
			label: org.display_name,
			value: org.id,
		})),
	},
};

export default meta;
type Story = StoryObj<typeof MultiSelectCombobox>;

export const Default: Story = {};

export const OpenCombobox: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByPlaceholderText("Select organization"));

		await waitFor(() =>
			expect(canvas.getByText("My Organization")).toBeInTheDocument(),
		);
	},
};

export const SelectComboboxItem: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByPlaceholderText("Select organization"));
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization" }),
		);
	},
};

export const SelectAllComboboxItems: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByPlaceholderText("Select organization"));
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization" }),
		);
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization 2" }),
		);

		await waitFor(() =>
			expect(
				canvas.getByText("All organizations selected"),
			).toBeInTheDocument(),
		);
	},
};

export const ClearFirstSelectedItem: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByPlaceholderText("Select organization"));
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization" }),
		);
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization 2" }),
		);
		await userEvent.click(canvas.getAllByTestId("clear-option-button")[0]);
	},
};

export const ClearAllComboboxItems: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByPlaceholderText("Select organization"));
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization" }),
		);
		await userEvent.click(canvas.getByTestId("clear-all-button"));

		await waitFor(() =>
			expect(
				canvas.getByPlaceholderText("Select organization"),
			).toBeInTheDocument(),
		);
	},
};
