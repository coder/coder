import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import {
	MockAIAuditTrailEvents,
	MockAIAuditTrailResponse,
} from "#/testHelpers/aiAuditTrail";
import { mockApiError } from "#/testHelpers/entities";
import AIActivityPage from "./AIActivityPage";

const meta: Meta<typeof AIActivityPage> = {
	title: "pages/AIActivityPage",
	component: AIActivityPage,
};

export default meta;
type Story = StoryObj<typeof AIActivityPage>;

export const Timeline: Story = {
	beforeEach: () => {
		API.getAIAuditTrailTimeline = async () => MockAIAuditTrailResponse;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("denied tcp pastebin.example.com:443 (x12)"),
		).toBeVisible();
		await expect(
			canvas.getByText("AI agent created in workspace"),
		).toBeVisible();
		// The creation entry was recorded after the fact, so both dates show.
		await expect(canvas.getByText(/^recorded /)).toBeVisible();
	},
};

export const Empty: Story = {
	beforeEach: () => {
		API.getAIAuditTrailTimeline = async () => ({ events: [], count: 0 });
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("No AI agent activity")).toBeVisible();
	},
};

export const LoadError: Story = {
	beforeEach: () => {
		API.getAIAuditTrailTimeline = async () => {
			throw mockApiError({
				message: "You are not authorized to view this owner's trail.",
			});
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(
				"You are not authorized to view this owner's trail.",
			),
		).toBeVisible();
	},
};

export const FilterByType: Story = {
	beforeEach: () => {
		API.getAIAuditTrailTimeline = async (filter) => {
			if (filter.types && filter.types.length > 0) {
				const events = MockAIAuditTrailEvents.filter((event) =>
					filter.types?.includes(event.type),
				);
				return { events, count: events.length };
			}
			return MockAIAuditTrailResponse;
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("denied tcp pastebin.example.com:443 (x12)"),
		).toBeVisible();

		await userEvent.click(canvas.getByRole("combobox", { name: "Event type" }));
		await userEvent.click(
			await within(document.body).findByRole("option", {
				name: "Credential use",
			}),
		);

		await expect(
			await canvas.findByText("credential presentation refused (password)"),
		).toBeVisible();
		await waitFor(async () => {
			await expect(
				canvas.queryByText("denied tcp pastebin.example.com:443 (x12)"),
			).not.toBeInTheDocument();
		});
	},
};

export const ChangeOwner: Story = {
	beforeEach: () => {
		API.getAIAuditTrailTimeline = async (filter) => {
			if (filter.owner === "alice") {
				return { events: [], count: 0 };
			}
			return MockAIAuditTrailResponse;
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("authorization granted"),
		).toBeVisible();

		const ownerInput = canvas.getByLabelText("Owner");
		await userEvent.clear(ownerInput);
		await userEvent.type(ownerInput, "alice");
		await userEvent.click(canvas.getByRole("button", { name: "Apply" }));

		await expect(await canvas.findByText("No AI agent activity")).toBeVisible();
	},
};
