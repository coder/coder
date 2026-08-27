import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { MockAIAuditAgent, MockAIAuditEvents } from "#/testHelpers/aiAudit";
import AIActivityPage from "./AIActivityPage";

const referenceDate = new Date("2026-08-25T12:00:00Z");

const meta = {
	title: "pages/AIActivityPage/AIActivityPage",
	component: AIActivityPage,
	args: {
		referenceDate,
	},
	beforeEach: () => {
		spyOn(API, "getAIAuditTimeline").mockResolvedValue({
			events: MockAIAuditEvents,
			count: MockAIAuditEvents.length,
		});
		spyOn(API, "getAIAuditAgents").mockResolvedValue([MockAIAuditAgent]);
	},
	parameters: {
		layout: "fullscreen",
	},
} satisfies Meta<typeof AIActivityPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API, "getAIAuditTimeline").mockImplementation(
			() => new Promise(() => {}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("status", { name: "Loading AI activity" }),
		).toBeVisible();
	},
};

export const Empty: Story = {
	beforeEach: () => {
		spyOn(API, "getAIAuditTimeline").mockResolvedValue({
			events: [],
			count: 0,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("No AI activity recorded"),
		).toBeVisible();
	},
};

export const Timeline: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const list = await canvas.findByRole("list", {
			name: "AI activity timeline",
		});
		const rows = within(list).getAllByRole("listitem");
		expect(rows).toHaveLength(MockAIAuditEvents.length);

		// Newest first: the resolved escalation leads, the sandbox start
		// closes the page.
		expect(rows[0]).toHaveTextContent("Escalation resolved");
		expect(rows[rows.length - 1]).toHaveTextContent("Sandbox started");

		// Agent IDs resolve to registry usernames.
		expect(rows[0]).toHaveTextContent(`Agent: ${MockAIAuditAgent.username}`);

		// The denied egress bucket keeps its aggregate count in the summary.
		expect(
			canvas.getByText("denied tcp evil.example.com:443 (x12)"),
		).toBeVisible();

		// Details stay collapsed until expanded.
		const deniedRow = rows[3];
		const detailBody = within(deniedRow).getByText(/"action": "denied"/);
		expect(detailBody).not.toBeVisible();
		await userEvent.click(within(deniedRow).getByText("Details"));
		expect(detailBody).toBeVisible();
	},
};

export const FilterByEventType: Story = {
	play: async ({ canvasElement, canvas }) => {
		await canvas.findByRole("list", { name: "AI activity timeline" });

		await userEvent.click(canvas.getByRole("combobox", { name: "Event type" }));
		// The listbox renders in a portal outside the canvas element.
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(await body.findByRole("option", { name: "Egress" }));

		await waitFor(() =>
			expect(API.getAIAuditTimeline).toHaveBeenCalledWith(
				expect.objectContaining({ types: ["egress"] }),
			),
		);
	},
};

export const FilterBySponsor: Story = {
	play: async ({ canvas }) => {
		await canvas.findByRole("list", { name: "AI activity timeline" });

		const sponsorInput = canvas.getByRole("textbox", { name: "Sponsor" });
		await userEvent.type(sponsorInput, "another-user{enter}");

		await waitFor(() =>
			expect(API.getAIAuditTimeline).toHaveBeenCalledWith(
				expect.objectContaining({ sponsor: "another-user" }),
			),
		);
		await waitFor(() =>
			expect(API.getAIAuditAgents).toHaveBeenCalledWith("another-user"),
		);
	},
};
