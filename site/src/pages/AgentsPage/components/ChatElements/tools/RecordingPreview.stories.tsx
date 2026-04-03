import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fireEvent, userEvent, waitFor, within } from "storybook/test";
import { RecordingPreview } from "./RecordingPreview";

// Static assets stored in site/.storybook/static/.
const TINY_MP4 = "/tiny-recording.mp4";
const TINY_THUMBNAIL = "/tiny-thumbnail.png";

const meta: Meta<typeof RecordingPreview> = {
	title: "pages/AgentsPage/ChatElements/tools/RecordingPreview",
	component: RecordingPreview,
	decorators: [
		(Story) => (
			<div className="max-w-3xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("img")).toBeInTheDocument();
		expect(
			canvas.getByRole("button", { name: "View recording" }),
		).toBeInTheDocument();
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
		const doc = canvasElement.ownerDocument;
		await waitFor(() => {
			const video = doc.querySelector("dialog video, [role='dialog'] video");
			expect(video).toBeInTheDocument();
			expect(video).toHaveAttribute("controls");
		});
	},
};

export const ThumbnailError: Story = {
	args: {
		recordingFileId: "dummy-recording-id",
		thumbnailFileId: "bad-thumb-id",
		src: TINY_MP4,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const img = canvasElement.querySelector("img");
		expect(img).not.toBeNull();
		fireEvent.error(img!);
		await waitFor(() => {
			expect(canvas.getByText("Thumbnail unavailable")).toBeInTheDocument();
			// The play button should still be available so the user can
			// attempt to view the recording even when the thumbnail fails.
			expect(
				canvas.getByRole("button", { name: "View recording" }),
			).toBeInTheDocument();
		});
	},
};

export const WithThumbnail: Story = {
	args: {
		recordingFileId: "rec-id",
		thumbnailFileId: "thumb-id",
		thumbnailSrc: TINY_THUMBNAIL,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const img = canvas.getByRole("img");
		expect(img).toBeInTheDocument();
		expect(img).toHaveAttribute("src", TINY_THUMBNAIL);
		// No <video> element should be in the DOM.
		expect(canvasElement.querySelector("video")).toBeNull();
		expect(
			canvas.getByRole("button", { name: "View recording" }),
		).toBeInTheDocument();
	},
};

export const WithoutThumbnail: Story = {
	args: {
		recordingFileId: "rec-id",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// No <img> or <video> element should be in the DOM.
		expect(canvasElement.querySelector("img")).toBeNull();
		expect(canvasElement.querySelector("video")).toBeNull();
		// Play button is still present.
		expect(
			canvas.getByRole("button", { name: "View recording" }),
		).toBeInTheDocument();
		// Gray placeholder div is visible.
		const placeholder = canvasElement.querySelector(
			".bg-surface-secondary:not(.flex)",
		);
		expect(placeholder).not.toBeNull();
	},
};
