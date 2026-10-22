import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import {
	MockNetworkPolicy,
	MockNetworkPolicyRevision,
} from "#/testHelpers/networkPolicies";
import { NetworkPolicyEditor } from "./NetworkPolicyEditor";

const mockHistoricalRevision = {
	...MockNetworkPolicyRevision,
	id: "80000000-0000-4000-8000-000000000002",
	created_at: "2026-10-19T12:00:00Z",
	hcl: `# Retained historical configuration\n${MockNetworkPolicyRevision.hcl}`,
};
const mockReadOnlyPolicy = {
	...MockNetworkPolicy,
	deletion_requested_at: "2026-10-21T12:00:00Z",
	revisions: [MockNetworkPolicyRevision, mockHistoricalRevision],
};

const meta = {
	title: "pages/NetworkPolicies/Editor",
	component: NetworkPolicyEditor,
	args: {
		policy: MockNetworkPolicy,
		selectedRevisionId: MockNetworkPolicyRevision.id,
		onSelectRevision: fn(),
		isPending: false,
		readOnly: false,
		error: undefined,
		onSave: fn(),
	},
} satisfies Meta<typeof NetworkPolicyEditor>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Saving: Story = { args: { isPending: true } };
export const ReadOnlyHistoricalVersion: Story = {
	args: {
		policy: mockReadOnlyPolicy,
		selectedRevisionId: mockHistoricalRevision.id,
		readOnly: true,
	},
};
export const ReadOnlyUnsavedChanges: Story = {
	render: function Render(args) {
		const [readOnly, setReadOnly] = useState(false);
		return (
			<NetworkPolicyEditor
				{...args}
				readOnly={readOnly}
				onSave={() => setReadOnly(true)}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			await canvas.findByRole("textbox", { name: "Policy HCL" }),
			"\n# Draft changes",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Save new version" }),
		);
	},
};
export const DiscardFocus: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			await canvas.findByRole("textbox", { name: "Policy HCL" }),
			"\n# Draft changes",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Discard edits" }),
		);
	},
};
export const ReadOnlyKeyboardFocus: Story = {
	args: ReadOnlyHistoricalVersion.args,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("textbox", { name: "Policy HCL" });
		canvas.getByRole("combobox", { name: "Version" }).focus();
		await userEvent.tab();
	},
};
export const ValidationError: Story = {
	args: {
		error: mockApiError({
			message: "The policy HCL is invalid.",
			validations: [{ field: "hcl", detail: "A trusted module is required." }],
		}),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("textbox", { name: "Policy HCL" }),
		);
		await userEvent.tab();
	},
};
export const KeyboardFocus: Story = {
	play: async ({ canvasElement }) => {
		(
			await within(canvasElement).findByRole("textbox", { name: "Policy HCL" })
		).focus();
	},
};
export const Mobile: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
};
