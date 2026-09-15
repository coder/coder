import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { VSCodeDevContainerButton } from "../VSCodeDevContainerButton/VSCodeDevContainerButton";
import { VSCodeDesktopButton } from "./VSCodeDesktopButton";

const meta: Meta<typeof VSCodeDesktopButton> = {
	title: "modules/resources/VSCodeDesktopButton",
	component: VSCodeDesktopButton,
};

export default meta;
type Story = StoryObj<typeof VSCodeDesktopButton>;

export const Default: Story = {
	args: {
		userName: MockWorkspace.owner_name,
		workspaceName: MockWorkspace.name,
		agentName: MockWorkspaceAgent.name,
		displayApps: [
			"vscode",
			"port_forwarding_helper",
			"ssh_helper",
			"vscode_insiders",
			"web_terminal",
		],
	},
};

/**
 * The variant choice is shared storage: picking a variant in one
 * button switches every consumer and persists for the next visit.
 */
export const SharedVariantSelection: Story = {
	args: Default.args,
	beforeEach: () => {
		localStorage.removeItem("vscode-variant");
	},
	render: (args) => (
		<div className="flex flex-col items-start gap-2">
			<VSCodeDesktopButton {...args} />
			<VSCodeDevContainerButton
				userName={args.userName}
				workspaceName={args.workspaceName}
				agentName={args.agentName}
				devContainerName="musing_ride"
				devContainerFolder="/workspace/coder"
				localWorkspaceFolder="/home/coder/coder"
				localConfigFile="/home/coder/coder/.devcontainer/devcontainer.json"
				displayApps={args.displayApps}
			/>
		</div>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getAllByRole("button", { name: "VS Code Desktop" }),
		).toHaveLength(2);

		const [desktopVariantToggle] = canvas.getAllByLabelText(
			"select VSCode variant",
		);
		await userEvent.click(desktopVariantToggle);
		// The dropdown renders in a portal outside the canvas.
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "VS Code Insiders" }),
		);

		await waitFor(() => {
			expect(
				canvas.getAllByRole("button", { name: "VS Code Insiders" }),
			).toHaveLength(2);
		});
		expect(localStorage.getItem("vscode-variant")).toBe("vscode-insiders");
	},
};
