import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockTemplate } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { TemplateSettingsForm } from "./TemplateSettingsForm";

const renameCheckbox = () =>
	screen.getByRole("checkbox", {
		name: /allow users to rename their workspaces/i,
	});

const renderForm = (
	overrides: Partial<Parameters<typeof TemplateSettingsForm>[0]> = {},
) => {
	const onSubmit = vi.fn();
	renderComponent(
		<TemplateSettingsForm
			template={MockTemplate}
			onSubmit={onSubmit}
			onCancel={vi.fn()}
			isSubmitting={false}
			accessControlEnabled
			advancedSchedulingEnabled
			portSharingControlsEnabled
			{...overrides}
		/>,
	);
	return { onSubmit };
};

describe("TemplateSettingsForm", () => {
	describe("allow_workspace_renames", () => {
		it("reflects the template's current value", () => {
			renderForm({
				template: { ...MockTemplate, allow_workspace_renames: true },
			});
			expect(renameCheckbox()).toBeChecked();
		});

		it("is unchecked when the template disallows renames", () => {
			renderForm({
				template: { ...MockTemplate, allow_workspace_renames: false },
			});
			expect(renameCheckbox()).not.toBeChecked();
		});

		it("submits the new value when toggled on", async () => {
			const user = userEvent.setup();
			const { onSubmit } = renderForm({
				template: { ...MockTemplate, allow_workspace_renames: false },
			});

			await user.click(renameCheckbox());
			await user.click(screen.getByRole("button", { name: /save/i }));

			await waitFor(() => {
				expect(onSubmit).toHaveBeenCalledWith(
					expect.objectContaining({ allow_workspace_renames: true }),
					// Formik passes its helpers as a second argument.
					expect.anything(),
				);
			});
		});

		it("submits the new value when toggled off", async () => {
			const user = userEvent.setup();
			const { onSubmit } = renderForm({
				template: { ...MockTemplate, allow_workspace_renames: true },
			});

			await user.click(renameCheckbox());
			await user.click(screen.getByRole("button", { name: /save/i }));

			await waitFor(() => {
				expect(onSubmit).toHaveBeenCalledWith(
					expect.objectContaining({ allow_workspace_renames: false }),
					expect.anything(),
				);
			});
		});
	});
});
