import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { MockOrganization, MockOrganization2 } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import {
	TEMPLATE_CUSTOMIZATIONS_FORM_ID,
	TemplateCustomizationsStep,
} from "./TemplateCustomizationsStep";
import { initialWizardState } from "./wizardState";

describe(TemplateCustomizationsStep.name, () => {
	it("describes invalid fields with their validation errors", async () => {
		const orgs = [MockOrganization, MockOrganization2];
		server.use(
			http.get("/api/v2/organizations", () => HttpResponse.json(orgs)),
			http.post("/api/v2/authcheck", () =>
				HttpResponse.json(Object.fromEntries(orgs.map((o) => [o.id, true]))),
			),
		);
		const onCreate = vi.fn();
		render(
			<>
				<TemplateCustomizationsStep
					state={{
						...initialWizardState,
						name: "docker",
						description: "a".repeat(129),
					}}
					onCreate={onCreate}
					onProvisionerStatusChange={vi.fn()}
				/>
				<button type="submit" form={TEMPLATE_CUSTOMIZATIONS_FORM_ID}>
					Create Template
				</button>
			</>,
		);

		const organization = await screen.findByRole("combobox", {
			name: /^Organization/,
		});
		await userEvent.click(
			screen.getByRole("button", { name: "Create Template" }),
		);

		await waitFor(() =>
			expect(organization).toHaveAccessibleDescription(
				"Select an organization to continue.",
			),
		);
		expect(
			screen.getByRole("textbox", { name: "Description" }),
		).toHaveAccessibleDescription(
			"Please enter a description that is less than or equal to 128 characters.",
		);
		expect(onCreate).not.toHaveBeenCalled();
	});
});
