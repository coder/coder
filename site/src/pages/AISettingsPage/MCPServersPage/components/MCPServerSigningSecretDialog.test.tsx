import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import { MCPServerSigningSecretDialog } from "./MCPServerSigningSecretDialog";

it("copies the secret and closes only after Done, not Escape", async () => {
	const user = userEvent.setup();
	const writeText = vi.spyOn(navigator.clipboard, "writeText");
	const onClose = vi.fn();
	const secret = "0123456789abcdef0123456789abcdef";
	render(<MCPServerSigningSecretDialog secret={secret} onClose={onClose} />);
	await user.click(screen.getByRole("button", { name: "Copy code" }));
	expect(writeText).toHaveBeenCalledWith(secret);
	await user.keyboard("{Escape}");
	expect(onClose).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: "Done" }));
	expect(onClose).toHaveBeenCalledOnce();
});
