import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
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

// Baseline: an enabled model with a connected provider renders the provider
// label and an "Enabled" badge.
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
// though the persisted model.enabled flag is true.
export const WithoutProviderForcesDisabled: Story = {
	args: {
		model: { ...mockClaude, enabled: true },
		providerLabel: "",
		hasProvider: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Unset")).toBeInTheDocument();
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();
		await expect(canvas.queryByText("Enabled")).not.toBeInTheDocument();
	},
};

// A disabled model with a provider keeps its provider label but stays
// "Disabled". This exercises the enabled=false path so the "Unset" wording
// is only tied to the missing provider case.
export const DisabledWithProvider: Story = {
	args: {
		model: { ...mockClaude, enabled: false, is_default: false },
		providerLabel: "Anthropic",
		hasProvider: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Anthropic")).toBeInTheDocument();
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();
		await expect(canvas.queryByText("Unset")).not.toBeInTheDocument();
	},
};
