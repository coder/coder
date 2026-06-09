import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { AIGatewayGuardrail } from "#/api/typesGenerated";
import { GuardrailsSection } from "./GuardrailsSection";

const mockGuardrail = (
	overrides: Partial<AIGatewayGuardrail> = {},
): AIGatewayGuardrail => ({
	id: "00000000-0000-0000-0000-000000000001",
	name: "presidio-pii",
	display_name: "Presidio PII masking",
	adapter_type: "presidio",
	active_version_id: "00000000-0000-0000-0000-0000000000a1",
	enabled: true,
	created_at: "2024-01-01T00:00:00Z",
	updated_at: "2024-01-01T00:00:00Z",
	versions: [
		{
			id: "00000000-0000-0000-0000-0000000000a1",
			guardrail_id: "00000000-0000-0000-0000-000000000001",
			version_number: 1,
			config: {},
			has_credential: false,
			description: "",
			created_at: "2024-01-01T00:00:00Z",
		},
	],
	...overrides,
});

const meta: Meta<typeof GuardrailsSection> = {
	title: "pages/AISettingsPage/GuardrailsSection",
	component: GuardrailsSection,
	args: {
		guardrails: [
			mockGuardrail(),
			mockGuardrail({
				id: "00000000-0000-0000-0000-000000000002",
				name: "secret-detection",
				display_name: "Secret detection",
				enabled: false,
			}),
		],
		isLoading: false,
		error: undefined,
		onCreate: fn(),
		isCreating: false,
		createError: undefined,
		onEdit: fn(),
		isEditing: false,
		editError: undefined,
		onDelete: fn(),
		deleteError: undefined,
		onToggle: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof GuardrailsSection>;

export const Default: Story = {};

export const Empty: Story = {
	args: { guardrails: [] },
};

export const Loading: Story = {
	args: { guardrails: [], isLoading: true },
};

export const ToggleEnabled: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const disableButtons = await canvas.findAllByRole("button", {
			name: "Disable",
		});
		await userEvent.click(disableButtons[0]);
		await expect(args.onToggle).toHaveBeenCalledWith(
			"00000000-0000-0000-0000-000000000001",
			false,
		);
	},
};
