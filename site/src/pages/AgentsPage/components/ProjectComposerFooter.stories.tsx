import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { MockChatProject, mockApiError } from "#/testHelpers/entities";
import { ProjectComposerFooter } from "./ProjectComposerFooter";

const meta: Meta<typeof ProjectComposerFooter> = {
	title: "pages/AgentsPage/ProjectComposerFooter",
	component: ProjectComposerFooter,
	args: { project: MockChatProject },
};

export default meta;
type Story = StoryObj<typeof ProjectComposerFooter>;

export const Default: Story = {};

export const EditDialogOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Edit project" }),
		);
	},
};

/** A failed save is cleared before the dialog opens again. */
export const ReopenedAfterFailedSave: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "updateChatProject").mockRejectedValue(
			mockApiError({ message: "Save failed" }),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(canvas.getByRole("button", { name: "Edit project" }));
		const dialog = await body.findByRole("dialog", { name: "Edit project" });
		await userEvent.type(
			within(dialog).getByRole("textbox", { name: /Name/ }),
			"!",
		);
		await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
		await within(dialog).findByText("Save failed");
		await userEvent.click(
			within(dialog).getByRole("button", { name: "Cancel" }),
		);
		// The open dialog hides the page from the accessibility tree.
		await userEvent.click(
			await canvas.findByRole("button", { name: "Edit project" }),
		);
		await body.findByRole("dialog", { name: "Edit project" });
	},
};
