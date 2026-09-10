import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { setupMatchMedia } from "#/testHelpers/matchMedia";
import {
	slotMachineStorageKey,
	useSlotMachineEasterEggListener,
} from "../../hooks/useSlotMachineEasterEgg";
import { LiveActivitySlot } from "./AssistantOutput";

const clearSlotMachineFlag = () => {
	localStorage.removeItem(slotMachineStorageKey);
};

const enableSlotMachineFlag = () => {
	localStorage.setItem(slotMachineStorageKey, "true");
	return clearSlotMachineFlag;
};

const meta: Meta<typeof LiveActivitySlot> = {
	title: "pages/AgentsPage/ChatConversation/LiveActivitySlot",
	component: LiveActivitySlot,
	beforeEach: () => {
		clearSlotMachineFlag();
		return clearSlotMachineFlag;
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
	beforeEach: enableSlotMachineFlag,
};

/** Reduced motion holds the reels on the landed row without spinning. */
export const SlotMachineReducedMotion: Story = {
	beforeEach: () => {
		const clearFlag = enableSlotMachineFlag();
		const { restore } = setupMatchMedia({
			"(prefers-reduced-motion: reduce)": true,
		});
		return () => {
			restore();
			clearFlag();
		};
	},
};

/**
 * Interrupting keeps its pause icon even while the easter egg is on so the
 * stop request stays legible.
 */
export const SlotMachineInterrupting: Story = {
	args: { interrupting: true },
	beforeEach: enableSlotMachineFlag,
};

const WithEasterEggListener = () => {
	useSlotMachineEasterEggListener();
	return <LiveActivitySlot />;
};

/** Typing the sequence with nothing focused turns the reels on. */
export const TypingSequenceTogglesSlotMachine: Story = {
	render: () => <WithEasterEggListener />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.keyboard("iddqd");
		await canvas.findByTestId("slot-machine-indicator");
	},
};
