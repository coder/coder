import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { WorkspaceFileChip } from "./WorkspaceFileChip";

const meta: Meta<typeof WorkspaceFileChip> = {
	title: "pages/AgentsPage/WorkspaceFileChip",
	component: WorkspaceFileChip,
	parameters: { layout: "centered" },
};
export default meta;
type Story = StoryObj<typeof WorkspaceFileChip>;

export const Uploaded: Story = {
	args: {
		name: "release.zip",
		size: 1024 * 1024 * 4,
		path: "/home/coder/.coder/chats/abcd1234/files/release.zip",
		onRemove: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("release.zip")).toBeInTheDocument();
		expect(canvas.getByText(/4\.0 MiB \u00b7 workspace/)).toBeInTheDocument();
		expect(
			canvas.getByRole("button", {
				name: "Copy workspace path for release.zip",
			}),
		).toBeInTheDocument();
		expect(
			canvas.getByRole("button", { name: "Remove release.zip" }),
		).toBeInTheDocument();
	},
};

export const Uploading: Story = {
	args: {
		name: "video.mp4",
		size: 1024 * 1024 * 12,
		isUploading: true,
		onRemove: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("video.mp4")).toBeInTheDocument();
		expect(canvas.getByText(/Uploading/)).toBeInTheDocument();
		// No copy button while uploading.
		expect(
			canvas.queryByRole("button", { name: /Copy workspace path/i }),
		).not.toBeInTheDocument();
	},
};

export const UploadError: Story = {
	args: {
		name: "huge.iso",
		size: 1024 * 1024 * 250,
		errorMessage: "File too large (250 MiB). Maximum is 100 MiB.",
		onRemove: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("huge.iso")).toBeInTheDocument();
		expect(canvas.getByText(/File too large \(250 MiB\)/)).toBeInTheDocument();
		// Copy button hidden in error state.
		expect(
			canvas.queryByRole("button", { name: /Copy workspace path/i }),
		).not.toBeInTheDocument();
	},
};

export const CopiesPathOnClick: Story = {
	args: {
		name: "notes.md",
		size: 2048,
		path: "/home/coder/.coder/chats/abcd1234/files/notes.md",
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		const writeText = fn().mockResolvedValue(undefined);
		Object.defineProperty(navigator, "clipboard", {
			value: { writeText },
			configurable: true,
		});
		await step("Click copy button", async () => {
			await userEvent.click(
				canvas.getByRole("button", {
					name: "Copy workspace path for notes.md",
				}),
			);
		});
		await waitFor(() => {
			expect(writeText).toHaveBeenCalledWith(
				"/home/coder/.coder/chats/abcd1234/files/notes.md",
			);
		});
	},
};

export const RemoveButtonInvokesCallback: Story = {
	args: {
		name: "todo.txt",
		size: 128,
		path: "/home/coder/.coder/chats/abcd1234/files/todo.txt",
		onRemove: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Remove todo.txt" }),
		);
		expect(args.onRemove).toHaveBeenCalledTimes(1);
	},
};

export const HidesCopyWhenPathOmitted: Story = {
	args: {
		name: "no-path.bin",
		size: 64,
		onRemove: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByRole("button", { name: /Copy workspace path/i }),
		).not.toBeInTheDocument();
	},
};

export const SmallFileShowsBytes: Story = {
	args: {
		name: "tiny.json",
		size: 768,
		path: "/home/coder/.coder/chats/abcd1234/files/tiny.json",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/768 B \u00b7 workspace/)).toBeInTheDocument();
	},
};
