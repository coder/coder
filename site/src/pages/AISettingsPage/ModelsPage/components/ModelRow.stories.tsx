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

// Control case: with an enabled provider and an enabled model, the row
// renders no status badge at all. Any regression that surfaces a badge for a
// healthy model shows up here.
export const WithProvider: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("OpenAI")).toBeInTheDocument();
		expect(canvas.queryByText("Enabled")).not.toBeInTheDocument();
		expect(canvas.queryByText("Disabled")).not.toBeInTheDocument();
		expect(canvas.queryByText("Unavailable")).not.toBeInTheDocument();
		await expect(canvas.queryByText("Unset")).not.toBeInTheDocument();
	},
};

// When the provider is missing (soft-deleted or otherwise unavailable) the
// Provider column shows "Unset" and the status cell shows an "Unavailable"
// notice even though the persisted model.enabled flag is true. An info icon
// next to the label reveals a tooltip explaining that the connected provider
// has been deleted.
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
		await expect(canvas.getByText("Unavailable")).toBeInTheDocument();
		expect(canvas.queryByText("Disabled")).not.toBeInTheDocument();

		const info = canvas.getByLabelText("Provider status");
		await userEvent.hover(info);
		const tooltip = await within(document.body).findByRole("tooltip");
		await expect(tooltip).toHaveTextContent(
			"The provider connected to this model has been deleted.",
		);
	},
};

// When the provider exists but is disabled, the label still renders (the
// provider is set) but the status cell shows the "Unavailable" notice with a
// tooltip because the model is not usable.
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
		const notice = canvas.getByText("Unavailable");
		await expect(notice).toBeInTheDocument();
		expect(canvas.queryByText("Disabled")).not.toBeInTheDocument();
		await expect(canvas.queryByText("Unset")).not.toBeInTheDocument();

		await userEvent.hover(notice);
		const tooltip = await within(document.body).findByRole("tooltip");
		await expect(tooltip).toHaveTextContent(
			"The provider connected to this model is disabled.",
		);
	},
};

// A disabled model with an enabled provider shows the "Disabled" badge next
// to the name (like the "Default" badge) with no status-cell notice. This
// exercises the enabled=false path so the "Unset" wording is only tied to
// the missing provider case.
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
		const nameCell = canvas.getByRole("cell", { name: /Claude Sonnet 4.5/ });
		await expect(within(nameCell).getByText("Disabled")).toBeInTheDocument();
		expect(canvas.queryByText("Unavailable")).not.toBeInTheDocument();
		await expect(canvas.queryByText("Unset")).not.toBeInTheDocument();
	},
};
