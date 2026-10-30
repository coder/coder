import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import { MCPTokenDialog } from "./MCPTokenDialog";

const renderDialog = (
	props: Partial<React.ComponentProps<typeof MCPTokenDialog>> = {},
) => {
	const onSave = vi.fn();
	const onRemove = vi.fn();
	render(
		<MCPTokenDialog
			serverName="Slack"
			audience="Your chats"
			hasCredential={false}
			isSaving={false}
			isRemoving={false}
			error={undefined}
			onSave={onSave}
			onRemove={onRemove}
			onClose={vi.fn()}
			{...props}
		/>,
	);
	return { onSave, onRemove, user: userEvent.setup() };
};

it("saves the trimmed token and keeps Save off until one is typed", async () => {
	const { onSave, user } = renderDialog();
	const save = screen.getByRole("button", { name: "Save token" });
	expect(save).toBeDisabled();

	await user.type(screen.getByLabelText("Token"), "  xoxb-secret ");
	await user.click(save);

	expect(onSave).toHaveBeenCalledWith("xoxb-secret");
});

it("offers to remove a stored token", async () => {
	const { onRemove, user } = renderDialog({ hasCredential: true });

	await user.click(screen.getByRole("button", { name: "Remove token" }));

	expect(onRemove).toHaveBeenCalledTimes(1);
});

describe("with a GitHub App", () => {
	const renderAppDialog = (current?: {
		app_id: number;
		installation_id: number;
		repositories: string[];
		use_for_git: boolean;
	}) => {
		const onSaveApp = vi.fn();
		const view = renderDialog({
			serverName: "GitHub",
			hasCredential: current !== undefined,
			githubApp: { current, onSave: onSaveApp },
		});
		return { ...view, onSaveApp };
	};

	it("saves the App with its key and a list of repositories", async () => {
		const { onSaveApp, user } = renderAppDialog();

		await user.click(screen.getByRole("radio", { name: "Use a GitHub App" }));
		await user.type(screen.getByLabelText("App ID"), "123");
		await user.type(screen.getByLabelText("Installation ID"), "456");
		await user.type(screen.getByLabelText("Private key"), "pem-text");
		await user.type(
			screen.getByLabelText(/Repositories/),
			"coder/coder, docs{Enter}site",
		);
		await user.click(
			screen.getByRole("checkbox", { name: /Also use this identity for git/ }),
		);
		await user.click(screen.getByRole("button", { name: "Save credential" }));

		expect(onSaveApp).toHaveBeenCalledWith({
			app_id: 123,
			installation_id: 456,
			private_key: "pem-text",
			repositories: ["coder/coder", "docs", "site"],
			use_for_git: true,
		});
	});

	it("uses the App for git across the whole installation when no repositories are listed", async () => {
		const { onSaveApp, user } = renderAppDialog({
			app_id: 123,
			installation_id: 456,
			repositories: [],
			use_for_git: false,
		});

		await user.click(
			screen.getByRole("checkbox", { name: /Also use this identity for git/ }),
		);
		await user.click(
			screen.getByRole("button", { name: "Replace credential" }),
		);

		expect(onSaveApp).toHaveBeenCalledWith(
			expect.objectContaining({ repositories: [], use_for_git: true }),
		);
	});

	it("needs a key for a new App", async () => {
		const { user } = renderAppDialog();

		await user.click(screen.getByRole("radio", { name: "Use a GitHub App" }));
		await user.type(screen.getByLabelText("App ID"), "123");
		await user.type(screen.getByLabelText("Installation ID"), "456");

		expect(
			screen.getByRole("button", { name: "Save credential" }),
		).toBeDisabled();
	});

	it("keeps the stored key only while the App ID is unchanged", async () => {
		const { onSaveApp, user } = renderAppDialog({
			app_id: 123,
			installation_id: 456,
			repositories: [],
			use_for_git: false,
		});
		const save = screen.getByRole("button", { name: "Replace credential" });

		await user.clear(screen.getByLabelText("Installation ID"));
		await user.type(screen.getByLabelText("Installation ID"), "789");
		await user.click(save);
		expect(onSaveApp).toHaveBeenCalledWith(
			expect.objectContaining({
				app_id: 123,
				installation_id: 789,
				private_key: undefined,
			}),
		);

		await user.clear(screen.getByLabelText("App ID"));
		await user.type(screen.getByLabelText("App ID"), "999");
		expect(save).toBeDisabled();
	});
});
