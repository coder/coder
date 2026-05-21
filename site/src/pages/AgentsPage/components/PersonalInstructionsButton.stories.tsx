import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { PersonalInstructionsButton } from "./PersonalInstructionsButton";

const ignoreResizeObserverLoopError = () => {
	const handleError = (event: ErrorEvent) => {
		if (
			event.message ===
			"ResizeObserver loop completed with undelivered notifications."
		) {
			event.stopImmediatePropagation();
			event.preventDefault();
		}
	};

	window.addEventListener("error", handleError);
	return () => window.removeEventListener("error", handleError);
};

const mockUserPrompt = (
	customPrompt = "",
	options: { updateError?: Error; updatePending?: boolean } = {},
) => {
	spyOn(API.experimental, "getUserChatCustomPrompt").mockResolvedValue({
		custom_prompt: customPrompt,
	});

	if (options.updatePending) {
		spyOn(API.experimental, "updateUserChatCustomPrompt").mockReturnValue(
			new Promise<TypesGen.UserChatCustomPrompt>(() => undefined),
		);
	} else if (options.updateError) {
		spyOn(API.experimental, "updateUserChatCustomPrompt").mockRejectedValue(
			options.updateError,
		);
	} else {
		spyOn(API.experimental, "updateUserChatCustomPrompt").mockImplementation(
			async (req) => ({ custom_prompt: req.custom_prompt }),
		);
	}

	return ignoreResizeObserverLoopError();
};

const openPopover = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	const trigger = canvas.getByRole("button", {
		name: "Edit personal instructions",
	});
	await userEvent.click(trigger);
	const body = within(canvasElement.ownerDocument.body);
	await body.findByText("Personal Instructions");
	return body;
};

const meta: Meta<typeof PersonalInstructionsButton> = {
	title: "pages/AgentsPage/PersonalInstructionsButton",
	component: PersonalInstructionsButton,
};

export default meta;
type Story = StoryObj<typeof PersonalInstructionsButton>;

export const Empty: Story = {
	beforeEach: () => mockUserPrompt(""),
};

export const WithExistingInstructions: Story = {
	beforeEach: () =>
		mockUserPrompt(
			"Always answer in TypeScript. Prefer arrow functions and avoid `any`.",
		),
};

export const OpensAndEditsInline: Story = {
	beforeEach: () => mockUserPrompt(""),
	play: async ({ canvasElement }) => {
		const body = await openPopover(canvasElement);

		const textarea = body.getByPlaceholderText(
			"Additional behavior, style, and tone preferences",
		);
		await userEvent.type(textarea, "Be concise and avoid filler words.");

		const save = body.getByRole("button", { name: "Save" });
		await userEvent.click(save);

		await waitFor(() => {
			expect(body.queryByText("Personal Instructions")).not.toBeInTheDocument();
		});
	},
};

export const ShowsErrorOnSaveFailure: Story = {
	beforeEach: () =>
		mockUserPrompt("", { updateError: new Error("network failure") }),
	play: async ({ canvasElement }) => {
		const body = await openPopover(canvasElement);

		const textarea = body.getByPlaceholderText(
			"Additional behavior, style, and tone preferences",
		);
		await userEvent.type(textarea, "Use British spelling.");

		await userEvent.click(body.getByRole("button", { name: "Save" }));

		await body.findByText("Failed to save personal instructions.");
	},
};
