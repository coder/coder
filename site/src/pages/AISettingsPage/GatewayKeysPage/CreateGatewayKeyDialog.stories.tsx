import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { CreateAIGatewayKeyResponse } from "#/api/typesGenerated";
import { MockCreateAIGatewayKeyResponse } from "#/testHelpers/entities";
import { CreateGatewayKeyDialog } from "./CreateGatewayKeyDialog";

const meta: Meta<typeof CreateGatewayKeyDialog> = {
	title: "pages/AISettingsPage/CreateGatewayKeyDialog",
	component: CreateGatewayKeyDialog,
	args: {
		open: true,
		onClose: fn(),
		onCreate: fn(
			(_name: string): Promise<CreateAIGatewayKeyResponse> =>
				Promise.resolve(MockCreateAIGatewayKeyResponse),
		),
	},
};

export default meta;
type Story = StoryObj<typeof CreateGatewayKeyDialog>;

export const Form: Story = {};

export const CreateAndReveal: Story = {
	play: async ({ args, canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const nameField = await body.findByLabelText("Name");
		await userEvent.type(nameField, "new-gateway");

		const createButton = await body.findByRole("button", { name: "Create" });
		await userEvent.click(createButton);

		await expect(args.onCreate).toHaveBeenCalledWith("new-gateway");
		await body.findByText(MockCreateAIGatewayKeyResponse.key);
	},
};
