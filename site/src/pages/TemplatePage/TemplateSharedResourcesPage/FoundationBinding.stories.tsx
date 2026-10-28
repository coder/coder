import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { MockTemplate, mockApiError } from "#/testHelpers/entities";
import { MockFoundation } from "#/testHelpers/templateResources";
import { FoundationBinding } from "./FoundationBinding";

const meta = {
	title: "pages/Foundations/Binding",
	component: FoundationBinding,
	args: { template: MockTemplate, canCreate: true, canBind: true },
	beforeEach: () => {
		const list = spyOn(API, "getFoundations").mockResolvedValue([
			{ ...MockFoundation, template_id: null },
		]);
		const auth = spyOn(API, "checkAuthorization").mockResolvedValue({
			canUseFoundation: true,
		});
		return () => {
			list.mockRestore();
			auth.mockRestore();
		};
	},
} satisfies Meta<typeof FoundationBinding>;
export default meta;
type Story = StoryObj<typeof meta>;
export const NoBinding: Story = {};
export const NoPermissions: Story = {
	args: { canCreate: false, canBind: false },
};
export const AttachOnly: Story = { args: { canCreate: false } };
export const Loading: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFoundations").mockImplementation(
			() => new Promise(() => {}),
		);
		return () => mock.mockRestore();
	},
};
export const ListFailure: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFoundations").mockRejectedValue(
			mockApiError({ message: "Could not load available Foundations." }),
		);
		return () => mock.mockRestore();
	},
};
export const CreateOnly: Story = { args: { canBind: false } };
export const Empty: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFoundations").mockResolvedValue([]);
		return () => mock.mockRestore();
	},
};
export const DelegationConfirmation: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("combobox", {
				name: "Available organization Foundations",
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: new RegExp(MockFoundation.name),
			}),
		);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Attach Foundation" }),
		);
	},
};
export const AttachFailure: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "setTemplateFoundation").mockRejectedValue(
			mockApiError({
				message: "This Foundation was attached to another template.",
			}),
		);
		return () => mock.mockRestore();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("combobox", {
				name: "Available organization Foundations",
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: new RegExp(MockFoundation.name),
			}),
		);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Attach Foundation" }),
		);
		await userEvent.click(
			await screen.findByRole("button", { name: "Confirm attachment" }),
		);
		await screen.findByRole("button", { name: "Retry attachment" });
	},
};
