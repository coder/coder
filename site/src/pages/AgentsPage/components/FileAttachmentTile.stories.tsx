import type { Meta, StoryObj } from "@storybook/react-vite";
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
};

export const UploadingWorkspaceFile: Story = {
	args: {
		name: "release.zip",
		size: 128_974_848,
		mediaType: "application/zip",
		metadataLabel: "workspace",
		isUploading: true,
	},
};

export const ErrorState: Story = {
	args: {
		name: "notes.txt",
		size: 512,
		mediaType: "text/plain",
		errorMessage: "Upload failed. Agent disconnected.",
	},
};
