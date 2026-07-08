import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { createMockFile } from "#/testHelpers/files";
import type { WorkspaceFileUpload } from "../hooks/useWorkspaceFileUploads";
import { WorkspaceUploadPreview } from "./WorkspaceUploadPreview";

const uploadedEntry = (name: string, size = 4096): WorkspaceFileUpload => ({
	id: `uploaded-${name}`,
	file: createMockFile(name, "application/zip"),
	status: "uploaded",
	response: {
		path: `/home/coder/.coder/chats/chat-1/files/${name}`,
		name,
		size,
		media_type: "application/zip",
		workspace_id: "ws-1",
	},
});

const meta: Meta<typeof WorkspaceUploadPreview> = {
	title: "pages/AgentsPage/WorkspaceUploadPreview",
	component: WorkspaceUploadPreview,
	args: {
		onRemove: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceUploadPreview>;

export const Uploaded: Story = {
	args: {
		uploads: [uploadedEntry("design-handoff.zip")],
	},
};

export const Queued: Story = {
	args: {
		uploads: [
			{
				id: "queued-1",
				file: createMockFile("bundle.zip", "application/zip"),
				status: "queued",
			},
		],
	},
};

export const Uploading: Story = {
	args: {
		uploads: [
			{
				id: "uploading-1",
				file: createMockFile("dataset.tar.gz", "application/gzip"),
				status: "uploading",
			},
		],
	},
};

export const UploadError: Story = {
	args: {
		uploads: [
			{
				id: "error-1",
				file: createMockFile("broken.zip", "application/zip"),
				status: "error",
				error: "Failed to upload file to workspace agent.",
			},
		],
	},
};

export const MixedStates: Story = {
	args: {
		uploads: [
			uploadedEntry("release.zip", 2 * 1024 * 1024),
			{
				id: "uploading-2",
				file: createMockFile("video.mp4", "video/mp4"),
				status: "uploading",
			},
			{
				id: "error-2",
				file: createMockFile("huge.iso", "application/x-iso9660-image"),
				status: "error",
				error: "Upload failed",
			},
		],
	},
};
