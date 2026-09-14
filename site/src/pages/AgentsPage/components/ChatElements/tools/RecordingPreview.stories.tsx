import type { Meta, StoryObj } from "@storybook/react-vite";
import { fireEvent, userEvent, within } from "storybook/test";
import { FileProbeProvider } from "../../ChatConversation/FileProbeContext";
import { RecordingPreview } from "./RecordingPreview";

// Static assets stored in site/.storybook/static/.
const TINY_MP4 = "/tiny-recording.mp4";
const TINY_THUMBNAIL = "/tiny-thumbnail.png";

const meta: Meta<typeof RecordingPreview> = {
	title: "pages/AgentsPage/ChatElements/tools/RecordingPreview",
	component: RecordingPreview,
};

export default meta;
type Story = StoryObj<typeof RecordingPreview>;

export const Default: Story = {
	args: {
		recordingFileId: "dummy-recording-id",
		thumbnailFileId: "dummy-thumb-id",
		thumbnailSrc: TINY_THUMBNAIL,
		src: TINY_MP4,
	},
};

export const LightboxOpen: Story = {
	args: {
		recordingFileId: "dummy-recording-id",
		thumbnailFileId: "dummy-thumb-id",
		thumbnailSrc: TINY_THUMBNAIL,
		src: TINY_MP4,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "View recording" }),
		);
	},
};

export const ThumbnailError: Story = {
	args: {
		recordingFileId: "dummy-recording-id",
		thumbnailFileId: "bad-thumb-id",
		src: TINY_MP4,
	},
	play: async ({ canvasElement }) => {
		const img = canvasElement.querySelector("img");
		fireEvent.error(img!);
	},
};

export const WithThumbnail: Story = {
	args: {
		recordingFileId: "rec-id",
		thumbnailFileId: "thumb-id",
		thumbnailSrc: TINY_THUMBNAIL,
	},
};

export const WithoutThumbnail: Story = {
	args: {
		recordingFileId: "rec-id",
	},
};

export const Expired: Story = {
	args: {
		recordingFileId: "evicted-rec-id",
		thumbnailFileId: "evicted-thumb-id",
	},
	decorators: [
		(Story) => (
			<FileProbeProvider evictedFileIds={new Set(["evicted-rec-id"])}>
				<Story />
			</FileProbeProvider>
		),
	],
};
