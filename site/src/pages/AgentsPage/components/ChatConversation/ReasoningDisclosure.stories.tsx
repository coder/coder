import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { ReasoningDisclosure } from "./ReasoningDisclosure";

const meta: Meta<typeof ReasoningDisclosure> = {
	title: "pages/AgentsPage/ChatConversation/ReasoningDisclosure",
	component: ReasoningDisclosure,
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl p-6">
				<Story />
			</div>
		),
	],
	args: {
		id: "reasoning-1",
		text: "Let me reason through this. The user wants me to explain the code they shared.",
		isStreaming: false,
	},
};

export default meta;
type Story = StoryObj<typeof ReasoningDisclosure>;

const REASONING_TEXT =
	"Let me reason through this. The user wants me to explain the code they shared.";

// Historical blocks start collapsed. The reasoning text is not
// rendered. The header shows a lightbulb and a "Thought" label and
// exposes aria-expanded=false.
export const CollapsedByDefault: Story = {
	args: {
		isStreaming: false,
		text: REASONING_TEXT,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const toggle = canvas.getByRole("button", { name: /thought/i });
		expect(toggle).toBeVisible();
		expect(toggle).toHaveAttribute("aria-expanded", "false");

		// Reasoning body is not in the DOM while collapsed.
		expect(canvas.queryByText(/let me reason through this/i)).toBeNull();

		// Lightbulb icon and chevron are present on the header.
		expect(
			canvasElement.querySelector('[data-testid="reasoning-icon"]'),
		).not.toBeNull();
		expect(
			canvasElement.querySelector('[data-testid="reasoning-chevron"]'),
		).not.toBeNull();
	},
};

// Clicking the header expands the block, revealing the reasoning text.
export const ClickToExpand: Story = {
	args: {
		isStreaming: false,
		text: REASONING_TEXT,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", { name: /thought/i });

		await userEvent.click(toggle);
		expect(toggle).toHaveAttribute("aria-expanded", "true");

		expect(canvas.getByText(/let me reason through this/i)).toBeVisible();
	},
};

// Keyboard users can toggle the disclosure with Space and Enter.
export const KeyboardToggle: Story = {
	args: {
		isStreaming: false,
		text: REASONING_TEXT,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", { name: /thought/i });

		toggle.focus();
		expect(toggle).toHaveFocus();

		await userEvent.keyboard(" ");
		expect(toggle).toHaveAttribute("aria-expanded", "true");

		await userEvent.keyboard("{Enter}");
		expect(toggle).toHaveAttribute("aria-expanded", "false");
	},
};

// Live-streaming blocks are expanded by default so users can watch
// the reasoning arrive.
export const ExpandedWhileStreaming: Story = {
	args: {
		isStreaming: true,
		text: REASONING_TEXT,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Header label is "Thinking" during streaming once any text
		// has arrived.
		const toggle = canvas.getByRole("button", { name: /thinking/i });
		expect(toggle).toHaveAttribute("aria-expanded", "true");

		expect(canvas.getByText(/^Thinking$/i)).toBeVisible();
	},
};

// During streaming with no reasoning text yet, the shimmer
// "Thinking..." affordance is visible inside the header button.
export const StreamingWithEmptyText: Story = {
	args: {
		isStreaming: true,
		text: "",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", { name: /thinking/i });
		// Target the header button explicitly so we catch a regression
		// where the shimmer drifts into the body instead of the header.
		expect(within(toggle).getByText("Thinking...")).toBeVisible();
	},
};

// The user can collapse a live-streaming block manually.
export const UserCollapsesDuringStream: Story = {
	args: {
		isStreaming: true,
		text: REASONING_TEXT,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Header label is "Thinking" during streaming.
		const toggle = canvas.getByRole("button", { name: /thinking/i });
		expect(toggle).toHaveAttribute("aria-expanded", "true");

		await userEvent.click(toggle);
		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText(/let me reason through this/i)).toBeNull();
	},
};

// Historical blocks with no reasoning text render an empty-state
// message inside the body.
export const CollapsedWithEmptyText: Story = {
	args: {
		isStreaming: false,
		text: "",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", { name: /thought/i });
		expect(toggle).toHaveAttribute("aria-expanded", "false");

		await userEvent.click(toggle);
		expect(canvas.getByText("No reasoning recorded.")).toBeVisible();
	},
};

// aria-controls on the toggle references a real DOM element so that
// assistive tech can associate the disclosure with its body.
export const AriaControlsLinkage: Story = {
	args: {
		isStreaming: false,
		text: REASONING_TEXT,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", { name: /thought/i });

		await userEvent.click(toggle);
		const controlsId = toggle.getAttribute("aria-controls");
		expect(controlsId).toBeTruthy();
		expect(canvasElement.querySelector(`#${controlsId}`)).not.toBeNull();
	},
};
