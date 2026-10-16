import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import { mockApiError } from "#/testHelpers/entities";
import { ErrorAlert } from "./ErrorAlert";

const mockError = mockApiError({
	message: "Email or password was invalid",
	detail: "Password is invalid",
});

const meta: Meta<typeof ErrorAlert> = {
	title: "components/Alert/ErrorAlert",
	component: ErrorAlert,
	args: {
		error: mockError,
		dismissible: false,
	},
};

export default meta;
type Story = StoryObj<typeof ErrorAlert>;

const ExampleAction = (
	<Button onClick={() => null} size="sm">
		Button
	</Button>
);

export const WithOnlyMessage: Story = {
	args: {
		error: mockApiError({
			message: "Email or password was invalid",
		}),
	},
};

export const APIErrorWithDetail: Story = {
	args: {
		error: mockApiError({
			message: "Magic dust is missing",
			detail: "without magic dust, the requested operation will never work",
		}),
	},
};

export const WithDismiss: Story = {
	args: {
		dismissible: true,
	},
};

export const WithAction: Story = {
	args: {
		actions: [ExampleAction],
	},
};

export const WithActionAndDismiss: Story = {
	args: {
		actions: [ExampleAction],
		dismissible: true,
	},
};

export const WithNonApiError: Story = {
	args: {
		error: new Error("Everything has gone horribly, devastatingly wrong."),
	},
};

const expandDetails: Story["play"] = async ({ canvasElement }) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: "Technical details" }),
	);
};

const requestErrorFields = {
	request: { method: "post", url: "/api/v2/workspaces" },
	status: 500,
	statusText: "Internal Server Error",
	headers: {
		"x-coder-request-id": "4f1c2a9e-7b3d-4e8a-9c21-5d6f0a1b2c3d",
		date: "Fri, 16 Oct 2026 17:00:00 GMT",
	},
};

export const WithTechnicalDetails: Story = {
	args: {
		error: mockApiError({
			message: "Failed to create workspace.",
			detail: "provisioner job failed: exit status 1",
			...requestErrorFields,
		}),
	},
	play: expandDetails,
};

export const WithResponseDataInTechnicalDetails: Story = {
	args: {
		error: mockApiError({
			message: "Failed to create workspace.",
			validations: [{ field: "name", detail: "Name is already taken." }],
			...requestErrorFields,
		}),
	},
	play: expandDetails,
};

export const WithTitle: Story = {
	args: {
		title: "We couldn't create this workspace",
		error: mockApiError({
			message:
				'Template "ubuntu-base" has been deprecated, and cannot be used to create a new workspace.',
			detail:
				"The ubuntu-base template is deprecated. Its admin says: \u201cUse ubuntu-24 instead. This template will be removed on June 30.\u201d",
			request: {
				method: "post",
				url: "/api/v2/organizations/coder/members/me/workspaces",
			},
			status: 400,
			statusText: "Bad Request",
			headers: {
				"x-coder-request-id": "3f9c2a1e-7b4d-4e8a-9c61-2d5f0b8e4a17",
				date: "Tue, 06 Oct 2026 14:14:00 GMT",
			},
		}),
	},
	play: expandDetails,
};

export const WithTitleWithoutDetail: Story = {
	args: {
		title: "We couldn't create this workspace",
		error: mockApiError({
			message:
				'Template "ubuntu-base" has been deprecated, and cannot be used to create a new workspace.',
			...requestErrorFields,
		}),
	},
	play: expandDetails,
};
