import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { FileAttachmentTile } from "./FileAttachmentTile";

const meta: Meta<typeof FileAttachmentTile> = {
	title: "pages/AgentsPage/FileAttachmentTile",
	component: FileAttachmentTile,
};

export default meta;
type Story = StoryObj<typeof FileAttachmentTile>;

export const RegularAttachment: Story = {
	args: {
		name: "design-notes.md",
		size: 18_432,
		mediaType: "text/markdown",
		href: "data:text/markdown;base64,IyBEZXNpZ24gbm90ZXM=",
		downloadName: "design-notes.md",
		onClick: fn(),
		onRemove: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const tile = canvas.getByRole("button", { name: "View design-notes.md" });

		expect(tile).toHaveTextContent("design-notes.md");
		expect(tile).toHaveTextContent("18 KiB");
		await userEvent.click(tile);
		expect(args.onClick).toHaveBeenCalledOnce();

		await userEvent.hover(tile);
		expect(
			canvas.getByRole("link", { name: "Download design-notes.md" }),
		).toHaveAttribute("download", "design-notes.md");
		await userEvent.click(
			canvas.getByRole("button", { name: "Remove design-notes.md" }),
		);
		expect(args.onRemove).toHaveBeenCalledOnce();
	},
};

export const WorkspaceFile: Story = {
	args: {
		name: "dataset.parquet",
		size: 1_073_741_824,
		mediaType: "application/octet-stream",
		metadataLabel: "workspace",
		copyPath: "/home/coder/.coder/chats/abcd1234/files/dataset.parquet",
	},
	play: async ({ canvasElement }) => {
		const writeText = fn(() => Promise.resolve());
		Object.defineProperty(navigator, "clipboard", {
			configurable: true,
			value: { writeText },
		});

		const canvas = within(canvasElement);
		const tile = canvas.getByTitle(
			"/home/coder/.coder/chats/abcd1234/files/dataset.parquet",
		);
		expect(tile).toHaveTextContent("dataset.parquet");
		expect(tile).toHaveTextContent("1 GiB");
		expect(tile).toHaveTextContent("workspace");

		await userEvent.hover(tile);
		await userEvent.click(
			canvas.getByRole("button", {
				name: "Copy workspace path for dataset.parquet",
			}),
		);
		await waitFor(() =>
			expect(writeText).toHaveBeenCalledWith(
				"/home/coder/.coder/chats/abcd1234/files/dataset.parquet",
			),
		);
		expect(canvas.getByRole("button", { name: "Copied path" })).toBeVisible();
	},
};

export const UploadingWorkspaceFile: Story = {
	args: {
		name: "release.zip",
		size: 128_974_848,
		mediaType: "application/zip",
		metadataLabel: "workspace",
		isUploading: true,
		copyPath: "/home/coder/.coder/chats/abcd1234/files/release.zip",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("release.zip")).toBeVisible();
		expect(canvas.getByText("Uploading 123 MiB")).toBeVisible();
		expect(
			canvas.queryByRole("button", {
				name: "Copy workspace path for release.zip",
			}),
		).not.toBeInTheDocument();
	},
};

export const ErrorState: Story = {
	args: {
		name: "notes.txt",
		size: 512,
		mediaType: "text/plain",
		errorMessage: "Upload failed. Agent disconnected.",
		onRemove: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("notes.txt")).toBeVisible();
		expect(
			canvas.getByText("Upload failed. Agent disconnected."),
		).toBeVisible();
		expect(canvas.getByRole("img", { name: "Upload error" })).toBeVisible();

		await userEvent.click(
			canvas.getByRole("button", { name: "Remove notes.txt" }),
		);
		expect(args.onRemove).toHaveBeenCalledOnce();
	},
};
