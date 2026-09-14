import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ImageLightbox } from "./ImageLightbox";

// Tiny 1x1 colored PNG so the lightbox has something visible to display.
const TINY_PNG =
	"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==";

const meta: Meta<typeof ImageLightbox> = {
	title: "components/ImageLightbox",
	component: ImageLightbox,
	decorators: [
		(Story) => (
			<div className="flex min-h-64 items-center justify-center p-8 text-content-primary">
				<p>Background content behind the lightbox overlay</p>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ImageLightbox>;

export const Default: Story = {
	args: {
		src: TINY_PNG,
		onClose: fn(),
	},
};
