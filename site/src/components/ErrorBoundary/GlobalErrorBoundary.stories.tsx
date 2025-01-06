import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent } from "@storybook/test";
import { within } from "@testing-library/react";
import type { ErrorResponse } from "react-router-dom";
import { GlobalErrorBoundaryInner } from "./GlobalErrorBoundary";

/**
 * React Router ErrorResponses have a "hidden" internal field that RR uses to
 * detect whether something is a loader error. The property doesn't exist in
 * the type information, but it does exist at runtime, and we need it to mock
 * out the story correctly
 */
type FullErrorResponse = Readonly<
	ErrorResponse & {
		internal: true;
	}
>;

const meta = {
	title: "components/GlobalErrorBoundary",
	component: GlobalErrorBoundaryInner,
} satisfies Meta<typeof GlobalErrorBoundaryInner>;

export default meta;
type Story = StoryObj<typeof meta>;

export const VanillaJavascriptError: Story = {
	args: {
		error: new Error("Something blew up :("),
	},
	play: async ({ canvasElement, args }) => {
		const error = args.error as Error;
		const canvas = within(canvasElement);
		const showErrorButton = canvas.getByRole("button", {
			name: /Show error/i,
		});
		await userEvent.click(showErrorButton);

		// Verify that error message content is now on screen; defer to
		// accessible name queries as much as possible
		canvas.getByRole("heading", { name: /Error/i });

		const p = canvas.getByTestId("description");
		expect(p).toHaveTextContent(error.message);

		const codeBlock = canvas.getByTestId("code");
		expect(codeBlock).toHaveTextContent(error.name);
		expect(codeBlock).toHaveTextContent(error.message);
	},
};

export const ReactRouterErrorResponse: Story = {
	args: {
		error: {
			internal: true,
			status: 500,
			statusText: "Aww, beans!",
			data: { message: "beans" },
		} satisfies FullErrorResponse,
	},
	play: async ({ canvasElement, args }) => {
		const error = args.error as FullErrorResponse;
		const canvas = within(canvasElement);
		const showErrorButton = canvas.getByRole("button", {
			name: /Show error/i,
		});
		await userEvent.click(showErrorButton);

		// Verify that error message content is now on screen; defer to
		// accessible name queries as much as possible
		const header = canvas.getByRole("heading", { name: /Aww, beans!/i });
		expect(header).toHaveTextContent(String(error.status));

		const codeBlock = canvas.getByTestId("code");
		const content = codeBlock.innerText;
		const parsed = JSON.parse(content);
		expect(parsed).toEqual(error.data);
	},
};

export const UnparsableError: Story = {
	args: {
		error: class WellThisIsDefinitelyWrong {},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const showErrorButton = canvas.queryByRole("button", {
			name: /Show error/i,
		});
		expect(showErrorButton).toBe(null);
	},
};
