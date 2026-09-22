import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import { MockCoderMCPServer } from "../testFixtures";
import { MCPServerForm } from "./MCPServerForm";

it.each(["", "replacement-signing-secret"])(
	"saves only a nonempty replacement signing secret: %s",
	async (secret) => {
		const user = userEvent.setup();
		const onUpdateServer = vi.fn(async () => undefined);
		render(
			<MCPServerForm
				server={MockCoderMCPServer}
				listPath="/servers"
				isSaving={false}
				isDeleting={false}
				canSelectUserOIDC
				onUpdateServer={onUpdateServer}
				onCancel={vi.fn()}
			/>,
		);
		await user.type(screen.getByLabelText(/display name/i), " Updated");
		await user.click(screen.getByRole("button", { name: /behavior/i }));
		const input = screen.getByLabelText("Signing secret");
		await user.click(input);
		if (secret) await user.type(input, secret);
		await user.click(screen.getByRole("button", { name: "Update server" }));
		await waitFor(() => {
			expect(onUpdateServer).toHaveBeenCalledWith(
				MockCoderMCPServer.id,
				expect.objectContaining({ signing_secret: secret || undefined }),
			);
		});
	},
);
