import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Table, TableBody } from "#/components/Table/Table";
import { mockClaude, mockGPT5 } from "../testFixtures";
import { ModelRow } from "./ModelRow";

const providerTypeByID = new Map<string, string>([
	["prov-openai", "openai"],
	["prov-anthropic", "anthropic"],
]);

const meta: Meta<typeof ModelRow> = {
	title: "pages/AISettingsPage/ModelsPage/ModelRow",
	component: ModelRow,
	args: {
		model: mockGPT5,
		providerLabel: "OpenAI",
		providerTypeByID,
		hasProvider: true,
		providerEnabled: true,
		onClick: () => {},
	},
	render: (args) => (
		<Table>
			<TableBody>
				<ModelRow {...args} />
			</TableBody>
		</Table>
	),
};

export default meta;
type Story = StoryObj<typeof ModelRow>;

// Control case for the effective-status logic: when both `hasProvider` and
// `providerEnabled` are true, the status badge must reflect the persisted
// enabled flag as-is. Any regression that inverts this collapses every model
// to "Disabled" in the list.
export const WithProvider: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("OpenAI")).toBeInTheDocument();
		await expect(canvas.getByText("Enabled")).toBeInTheDocument();
		await expect(canvas.queryByText("Unset")).not.toBeInTheDocument();
	},
};

// When the provider is missing (soft-deleted or otherwise unavailable) the
// Provider column shows "Unset" and the status collapses to "Disabled" even
// though the persisted model.enabled flag is true. An info icon next to the
// label reveals a tooltip explaining that the connected provider has been
// deleted.
export const WithoutProviderForcesDisabled: Story = {
	args: {
		model: { ...mockClaude, enabled: true },
		providerLabel: "",
		hasProvider: false,
		providerEnabled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Unset")).toBeInTheDocument();
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();
		await expect(canvas.queryByText("Enabled")).not.toBeInTheDocument();

		const info = canvas.getByLabelText("Provider status");
		await userEvent.hover(info);
		const tooltip = await within(document.body).findByRole("tooltip");
		await expect(tooltip).toHaveTextContent(
			"The provider connected to this model has been deleted.",
		);
	},
};

// When the provider exists but is disabled, the label still renders (the
// provider is set) but the status collapses to "Disabled" because the model
// is not usable.
export const DisabledProviderForcesDisabled: Story = {
	args: {
		model: { ...mockClaude, enabled: true, is_default: false },
		providerLabel: "Anthropic",
		hasProvider: true,
		providerEnabled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Anthropic")).toBeInTheDocument();
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();
		await expect(canvas.queryByText("Enabled")).not.toBeInTheDocument();
		await expect(canvas.queryByText("Unset")).not.toBeInTheDocument();
	},
};

// A disabled model with an enabled provider keeps its provider label but
// stays "Disabled". This exercises the enabled=false path so the "Unset"
// wording is only tied to the missing provider case.
export const DisabledModelWithProvider: Story = {
	args: {
		model: { ...mockClaude, enabled: false, is_default: false },
		providerLabel: "Anthropic",
		hasProvider: true,
		providerEnabled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Anthropic")).toBeInTheDocument();
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();
		await expect(canvas.queryByText("Unset")).not.toBeInTheDocument();
	},
};
