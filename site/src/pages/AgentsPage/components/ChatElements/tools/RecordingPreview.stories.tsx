import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fireEvent, userEvent, waitFor, within } from "storybook/test";
import { RecordingPreview } from "./RecordingPreview";

// The file is stored in site/.storybook/static/tiny-recording.mp4.
const TINY_MP4 = "/tiny-recording.mp4";

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
		src: TINY_MP4,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: "View recording" }),
		).toBeInTheDocument();
	},
};

export const LightboxOpen: Story = {
	args: {
		recordingFileId: "dummy-recording-id",
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
		src: TINY_MP4,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const video = canvasElement.querySelector("video");
		expect(video).not.toBeNull();
		fireEvent.error(video!);
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
