import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { AttachmentPreview, type UploadState } from "./AttachmentPreview";

// Tiny 1x1 transparent PNG as data URI for previews.
const TINY_PNG =
	"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==";

const createMockFile = (name: string, type: string, size = 9) =>
	new File([new Uint8Array(size)], name, { type });

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
		onTextPreview: fn(),
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
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const thumbnail = await canvas.findByRole("img", { name: "photo.png" });
		expect(thumbnail).toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button", { name: "photo.png" }));
		expect(args.onPreview).toHaveBeenCalledWith(TINY_PNG);
	},
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByTitle("Loading spinner")).toBeInTheDocument();
	},
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const overlay = canvas.getByLabelText("Upload error");
		expect(overlay).toBeInTheDocument();
		await userEvent.hover(overlay);
		// After hover, the tooltip renders the error message. Use
		// getAllByText because the text appears in both the tooltip
		// trigger overlay and the tooltip content popover.
		const body = within(canvasElement.ownerDocument.body);
		const matches = await body.findAllByText(/Upload failed: server error/i);
		expect(matches.length).toBeGreaterThanOrEqual(1);
	},
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

export const TextAttachment: Story = {
	args: (() => {
		const file = createMockFile("clipboard.txt", "text/plain", 2048);
		return {
			attachments: [file],
			uploadStates: new Map<File, UploadState>([
				[file, { status: "uploaded", fileId: "file-1" }],
			]),
			previewUrls: new Map<File, string>(),
			textContents: new Map<File, string>([
				[
					file,
					"This is the pasted text content.\nIt has multiple lines.\nAnd should be displayed in a readable card format.",
				],
			]),
		};
	})(),
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const textCard = await canvas.findByRole("button", {
			name: "View clipboard.txt",
		});
		expect(textCard).toHaveTextContent(/This is the pasted text content\./i);
		await userEvent.click(textCard);
		expect(args.onTextPreview).toHaveBeenCalledWith(
			"This is the pasted text content.\nIt has multiple lines.\nAnd should be displayed in a readable card format.",
			"clipboard.txt",
		);
	},
};

export const ThreeTextAttachments: Story = {
	args: (() => {
		const file1 = createMockFile("paste-1.txt", "text/plain", 2048);
		const file2 = createMockFile("paste-2.txt", "text/plain", 3072);
		const file3 = createMockFile("paste-3.txt", "text/plain", 1024);
		return {
			attachments: [file1, file2, file3],
			uploadStates: new Map<File, UploadState>([
				[file1, { status: "uploaded", fileId: "file-1" }],
				[file2, { status: "uploaded", fileId: "file-2" }],
				[file3, { status: "uploaded", fileId: "file-3" }],
			]),
			previewUrls: new Map<File, string>(),
			textContents: new Map<File, string>([
				[
					file1,
					"First pasted document with several lines of content.\nLine 2 of the first document.\nLine 3 continues here.",
				],
				[
					file2,
					"Second pasted text is a log file:\n[INFO] Server started on port 8080\n[WARN] Memory usage at 85%\n[ERROR] Connection timeout after 30s",
				],
				[
					file3,
					"Third paste is a short config:\nhost=localhost\nport=5432\ndb=myapp",
				],
			]),
		};
	})(),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findAllByRole("button", { name: /View paste-[1-3]\.txt/ }),
		).toHaveLength(3);
		expect(
			canvas.getByText(
				/First pasted document with several lines of content\./i,
			),
		).toBeInTheDocument();
	},
};

export const ThreeMixedAttachments: Story = {
	args: (() => {
		const imageFile = createMockFile("screenshot.png", "image/png");
		const textFile1 = createMockFile("logs.txt", "text/plain", 2048);
		const textFile2 = createMockFile("config.txt", "text/plain", 1024);
		return {
			attachments: [imageFile, textFile1, textFile2],
			uploadStates: new Map<File, UploadState>([
				[imageFile, { status: "uploaded", fileId: "img-1" }],
				[textFile1, { status: "uploaded", fileId: "txt-1" }],
				[textFile2, { status: "uploaded", fileId: "txt-2" }],
			]),
			previewUrls: new Map<File, string>([[imageFile, TINY_PNG]]),
			textContents: new Map<File, string>([
				[
					textFile1,
					"[2025-01-15 10:30:00] Application started\n[2025-01-15 10:30:01] Connected to database\n[2025-01-15 10:30:02] Listening on :8080",
				],
				[
					textFile2,
					"DATABASE_URL=postgres://localhost/myapp\nREDIS_URL=redis://localhost:6379\nSECRET_KEY=abc123",
				],
			]),
		};
	})(),
};

export const MixedImageAndText: Story = {
	args: (() => {
		const imageFile = createMockFile("photo.png", "image/png");
		const textFile = createMockFile("clipboard.txt", "text/plain", 2048);
		return {
			attachments: [imageFile, textFile],
			uploadStates: new Map<File, UploadState>([
				[imageFile, { status: "uploaded", fileId: "file-1" }],
				[textFile, { status: "uploaded", fileId: "file-2" }],
			]),
			previewUrls: new Map<File, string>([[imageFile, TINY_PNG]]),
			textContents: new Map<File, string>([
				[
					textFile,
					"This is some pasted text content that appears alongside an image attachment.",
				],
			]),
		};
	})(),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByRole("img", { name: "photo.png" }),
		).toBeInTheDocument();
		expect(
			canvas.getByRole("button", { name: "View clipboard.txt" }),
		).toBeInTheDocument();
	},
};

export const TextAttachmentUploading: Story = {
	args: (() => {
		const file = createMockFile("clipboard.txt", "text/plain", 2048);
		return {
			attachments: [file],
			uploadStates: new Map<File, UploadState>([
				[file, { status: "uploading" }],
			]),
			previewUrls: new Map<File, string>(),
			textContents: new Map<File, string>([
				[file, "Uploading text content..."],
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
