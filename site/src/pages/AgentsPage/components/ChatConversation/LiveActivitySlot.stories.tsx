import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Toaster } from "#/components/Toaster/Toaster";
import { setupMatchMedia } from "#/testHelpers/matchMedia";
import {
	slotMachineStorageKey,
	useSlotMachineEasterEggListener,
} from "../../hooks/useSlotMachineEasterEgg";
import { LiveActivitySlot } from "./AssistantOutput";

const meta: Meta<typeof LiveActivitySlot> = {
	title: "pages/AgentsPage/ChatConversation/LiveActivitySlot",
	component: LiveActivitySlot,
	beforeEach: () => {
		localStorage.removeItem(slotMachineStorageKey);
	},
};
export default meta;
type Story = StoryObj<typeof LiveActivitySlot>;

export const Default: Story = {};

export const Interrupting: Story = {
	args: { interrupting: true },
};

/** The easter egg flag swaps the lightbulb and shimmer for the reels. */
export const SlotMachineActive: Story = {
	beforeEach: () => {
		localStorage.setItem(slotMachineStorageKey, "true");
	},
};

/** Reduced motion holds the reels on the landed row without spinning. */
export const SlotMachineReducedMotion: Story = {
	beforeEach: () => {
		localStorage.setItem(slotMachineStorageKey, "true");
		const { restore } = setupMatchMedia({
			"(prefers-reduced-motion: reduce)": true,
		});
		return restore;
	},
};

/**
 * Interrupting keeps its pause icon even while the easter egg is on so the
 * stop request stays legible.
 */
export const SlotMachineInterrupting: Story = {
	args: { interrupting: true },
	beforeEach: () => {
		localStorage.setItem(slotMachineStorageKey, "true");
	},
};

const WithEasterEggListener = () => {
	useSlotMachineEasterEggListener();
	return (
		<>
			<LiveActivitySlot />
			<Toaster />
		</>
	);
};

/** Typing the sequence with nothing focused toggles the flag and persists it. */
export const TypingSequenceTogglesSlotMachine: Story = {
	render: () => <WithEasterEggListener />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.keyboard("iddqd");
		await canvas.findByTestId("slot-machine-indicator");
		expect(localStorage.getItem(slotMachineStorageKey)).toBe("true");
	},
};
