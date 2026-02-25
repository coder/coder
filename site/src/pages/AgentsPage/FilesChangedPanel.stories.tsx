import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ChatDiffStatusResponse } from "api/api";
import { API } from "api/api";
import type { ChatDiffContents } from "api/typesGenerated";
import { expect, screen, spyOn } from "storybook/test";
import { FilesChangedPanel } from "./FilesChangedPanel";

const defaultDiffStatus: ChatDiffStatusResponse = {
	chat_id: "test-chat",
	changes_requested: false,
	additions: 0,
	deletions: 0,
	changed_files: 0,
};

const defaultDiffContents: ChatDiffContents = {
	chat_id: "test-chat",
};

const meta: Meta<typeof FilesChangedPanel> = {
	title: "pages/AgentsPage/FilesChangedPanel",
	component: FilesChangedPanel,
	args: {
		chatId: "test-chat",
	},
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue(defaultDiffStatus);
		spyOn(API, "getChatDiffContents").mockResolvedValue(defaultDiffContents);
	},
};

export default meta;
type Story = StoryObj<typeof FilesChangedPanel>;

export const EmptyDiff: Story = {
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: undefined,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: "",
		});
	},
	play: async () => {
		await screen.findByText("No file changes to display.");
		expect(screen.getByText("No file changes to display.")).toBeInTheDocument();
	},
};

export const ParseError: Story = {
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/123",
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: "not-a-valid-unified-diff",
		});
	},
	play: async () => {
		await screen.findByText("No file changes to display.");
		expect(screen.getByText("No file changes to display.")).toBeInTheDocument();
	},
};
