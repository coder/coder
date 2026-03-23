import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { AttachmentPreview, type UploadState } from "./AgentChatInput";

// Tiny 1x1 transparent PNG as data URI for previews.
const TINY_PNG =
	"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==";

const createMockFile = (name: string, type: string) =>
	new File(["mock-data"], name, { type });

const meta: Meta<typeof AttachmentPreview> = {
	title: "pages/AgentsPage/AttachmentPreview",
	component: AttachmentPreview,
	decorators: [
		(Story) => (
			<div className="max-w-xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
	args: {
		onRemove: fn(),
		onPreview: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AttachmentPreview>;

export const SingleImage: Story = {
	args: (() => {
		const file = createMockFile("photo.png", "image/png");
		return {
			attachments: [file],
			uploadStates: new Map<File, UploadState>([
				[file, { status: "uploaded", fileId: "file-1" }],
			]),
			previewUrls: new Map<File, string>([[file, TINY_PNG]]),
		};
	})(),
};

export const MultipleImages: Story = {
	args: (() => {
		const files = [
			createMockFile("photo-1.png", "image/png"),
			createMockFile("photo-2.jpg", "image/jpeg"),
			createMockFile("photo-3.png", "image/png"),
		];
		return {
			attachments: files,
			uploadStates: new Map<File, UploadState>(
				files.map((f) => [f, { status: "uploaded", fileId: f.name }]),
			),
			previewUrls: new Map<File, string>(files.map((f) => [f, TINY_PNG])),
		};
	})(),
};

export const Uploading: Story = {
	args: (() => {
		const file = createMockFile("uploading.png", "image/png");
		return {
			attachments: [file],
			uploadStates: new Map<File, UploadState>([
				[file, { status: "uploading" }],
			]),
			previewUrls: new Map<File, string>([[file, TINY_PNG]]),
		};
	})(),
};

export const UploadError: Story = {
	args: (() => {
		const file = createMockFile("broken.png", "image/png");
		return {
			attachments: [file],
			uploadStates: new Map<File, UploadState>([
				[file, { status: "error", error: "Upload failed: server error" }],
			]),
			previewUrls: new Map<File, string>([[file, TINY_PNG]]),
		};
	})(),
};

export const FileTooLarge: Story = {
	args: (() => {
		const file = createMockFile("huge-screenshot.png", "image/png");
		return {
			attachments: [file],
			uploadStates: new Map<File, UploadState>([
				[
					file,
					{
						status: "error",
						error: "File too large (12.4 MB). Maximum is 10 MB.",
					},
				],
			]),
			previewUrls: new Map<File, string>([[file, TINY_PNG]]),
		};
	})(),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const overlay = canvas.getByLabelText("Upload error");
		await userEvent.hover(overlay);
	},
};

export const NonImageFile: Story = {
	args: (() => {
		const file = createMockFile("readme.txt", "text/plain");
		return {
			attachments: [file],
			uploadStates: new Map<File, UploadState>([
				[file, { status: "uploaded", fileId: "file-txt" }],
			]),
		};
	})(),
};

export const MixedStates: Story = {
	args: (() => {
		const uploaded = createMockFile("done.png", "image/png");
		const uploading = createMockFile("pending.jpg", "image/jpeg");
		const errored = createMockFile("failed.png", "image/png");
		const attachments = [uploaded, uploading, errored];
		return {
			attachments,
			uploadStates: new Map<File, UploadState>([
				[uploaded, { status: "uploaded", fileId: "file-ok" }],
				[uploading, { status: "uploading" }],
				[errored, { status: "error", error: "Network timeout" }],
			]),
			previewUrls: new Map<File, string>([
				[uploaded, TINY_PNG],
				[uploading, TINY_PNG],
				[errored, TINY_PNG],
			]),
		};
	})(),
};
