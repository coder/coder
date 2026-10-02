import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { AgentSSHButton } from "./SSHButton";

it("does not autofocus the copy button when opening SSH instructions", async () => {
	const user = userEvent.setup();
	const writeText = vi
		.spyOn(navigator.clipboard, "writeText")
		.mockResolvedValue();
	render(
		<AgentSSHButton
			workspaceName={MockWorkspace.name}
			agentName={MockWorkspaceAgent.name}
			workspaceOwnerUsername={MockWorkspace.owner_name}
		/>,
	);

	const trigger = screen.getByRole("button", { name: "Connect via SSH" });
	await user.click(trigger);
	const dialog = await screen.findByRole("dialog");
	const [configureSSH, connectSSH] = within(dialog).getAllByRole("button", {
		name: "Copy code",
	});
	expect(configureSSH).not.toHaveFocus();

	await user.tab();
	expect(configureSSH).toHaveFocus();
	await user.keyboard("{Enter}");
	expect(writeText).toHaveBeenCalledWith("coder config-ssh");
	await user.tab();
	expect(connectSSH).toHaveFocus();
	await user.tab();
	expect(
		within(dialog).getByRole("link", { name: "Install Coder CLI" }),
	).toHaveFocus();
	await user.keyboard("{Escape}");
	await waitFor(() => expect(trigger).toHaveFocus());
});
