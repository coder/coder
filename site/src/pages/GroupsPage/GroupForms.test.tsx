import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockGroup } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { CreateGroupPageView } from "./CreateGroupPageView";
import GroupSettingsPageView from "./GroupSettingsPageView";

describe.each(["create", "edit"] as const)("%s group name", (mode) => {
	it.each([33, 255])("submits a %i-character name", async (length) => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const name = "a".repeat(length);
		render(
			mode === "create" ? (
				<CreateGroupPageView
					onSubmit={onSubmit}
					onCancel={vi.fn()}
					isLoading={false}
					showOrganizations
				/>
			) : (
				<GroupSettingsPageView
					group={{ ...MockGroup, name }}
					onSubmit={onSubmit}
					showAISettings={false}
					initialBudgetDollars={null}
					formErrors={undefined}
					isUpdating={false}
				/>
			),
		);
		await act(async () => {
			if (mode === "create") {
				await user.click(screen.getByRole("textbox", { name: /^Name/ }));
				await user.paste(name);
			}
			await user.click(screen.getByRole("button", { name: "Save" }));
		});
		await waitFor(() => {
			expect(onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({ name }),
				expect.anything(),
			);
		});
	});
});
