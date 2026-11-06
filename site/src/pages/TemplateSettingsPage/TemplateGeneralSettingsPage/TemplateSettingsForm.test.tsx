import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { MockTemplate } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { TemplateSettingsForm } from "./TemplateSettingsForm";

const renameCheckbox = () =>
	screen.getByRole("checkbox", {
		name: /allow users to rename their workspaces/i,
	});

const moduleCacheCheckbox = () =>
	screen.getByRole("checkbox", {
		name: /disable terraform module caching/i,
	});

const renderForm = (
	overrides: Partial<Parameters<typeof TemplateSettingsForm>[0]> = {},
) => {
	const onSubmit = vi.fn();
	renderComponent(
		<MemoryRouter>
			<TemplateSettingsForm
				template={MockTemplate}
				onSubmit={onSubmit}
				onCancel={vi.fn()}
				isSubmitting={false}
				accessControlEnabled
				advancedSchedulingEnabled
				portSharingControlsEnabled
				canViewPremium
				{...overrides}
			/>
		</MemoryRouter>,
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

	describe("disable_module_cache", () => {
		it("submits the new value when toggled on", async () => {
			const user = userEvent.setup();
			const { onSubmit } = renderForm();

			await user.click(moduleCacheCheckbox());
			await user.click(screen.getByRole("button", { name: /save/i }));

			await waitFor(() => {
				expect(onSubmit).toHaveBeenCalledWith(
					expect.objectContaining({ disable_module_cache: true }),
					expect.anything(),
				);
			});
		});

		it("is checked and read-only when the deployment disables caching", async () => {
			const user = userEvent.setup();
			renderForm({
				template: {
					...MockTemplate,
					module_cache_disabled_by_deployment: true,
				},
			});

			const checkbox = moduleCacheCheckbox();
			expect(checkbox).toBeChecked();
			expect(checkbox).toBeDisabled();

			await user.click(checkbox);
			expect(checkbox).toBeChecked();
		});
	});

	describe("Premium notices", () => {
		it("link to the Premium page for users who can view licenses", () => {
			renderForm({
				accessControlEnabled: false,
				advancedSchedulingEnabled: false,
				portSharingControlsEnabled: false,
			});

			const links = screen.getAllByRole("link", {
				name: /learn about premium/i,
			});
			expect(links).toHaveLength(3);
			for (const link of links) {
				expect(link).toHaveAttribute("href", "/deployment/premium");
			}
		});

		it("omit the link for users who cannot view licenses", () => {
			renderForm({
				accessControlEnabled: false,
				advancedSchedulingEnabled: false,
				portSharingControlsEnabled: false,
				canViewPremium: false,
			});

			expect(
				screen.getAllByText(/premium license is required/i).length,
			).toBeGreaterThan(0);
			expect(
				screen.queryByRole("link", { name: /learn about premium/i }),
			).not.toBeInTheDocument();
		});
	});
});
