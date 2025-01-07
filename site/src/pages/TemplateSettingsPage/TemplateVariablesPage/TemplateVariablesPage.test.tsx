import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import {
	MockTemplate,
	MockTemplateVersion,
	MockTemplateVersion2,
	MockTemplateVersionVariable1,
	MockTemplateVersionVariable2,
} from "testHelpers/entities";
import {
	renderWithTemplateSettingsLayout,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { delay } from "utils/delay";
import TemplateVariablesPage from "./TemplateVariablesPage";

const validFormValues = {
	first_variable: "Hello world",
	second_variable: "123",
};

const renderTemplateVariablesPage = async () => {
	renderWithTemplateSettingsLayout(<TemplateVariablesPage />, {
		route: `/templates/${MockTemplate.name}/variables`,
		path: "/templates/:template/variables",
		extraRoutes: [{ path: `/templates/${MockTemplate.name}`, element: <></> }],
	});
	await waitForLoaderToBeRemoved();
};

describe("TemplateVariablesPage", () => {
	it("renders with variables", async () => {
		jest.spyOn(API, "getTemplateByName").mockResolvedValueOnce(MockTemplate);
		jest
			.spyOn(API, "getTemplateVersion")
			.mockResolvedValueOnce(MockTemplateVersion);
		jest
			.spyOn(API, "getTemplateVersionVariables")
			.mockResolvedValueOnce([
				MockTemplateVersionVariable1,
				MockTemplateVersionVariable2,
			]);

		await renderTemplateVariablesPage();

		const firstVariable = await screen.findByLabelText(
			MockTemplateVersionVariable1.name,
		);
		expect(firstVariable).toBeDefined();

		const secondVariable = await screen.findByLabelText(
			MockTemplateVersionVariable2.name,
		);
		expect(secondVariable).toBeDefined();
	});

	it("user submits the form successfully", async () => {
		jest.spyOn(API, "getTemplateByName").mockResolvedValueOnce(MockTemplate);
		jest
			.spyOn(API, "getTemplateVersion")
			.mockResolvedValue(MockTemplateVersion);
		jest
			.spyOn(API, "getTemplateVersionVariables")
			.mockResolvedValueOnce([
				MockTemplateVersionVariable1,
				MockTemplateVersionVariable2,
			]);
		jest
			.spyOn(API, "createTemplateVersion")
			.mockResolvedValueOnce(MockTemplateVersion2);
		jest.spyOn(API, "updateActiveTemplateVersion").mockResolvedValueOnce({
			message: "done",
		});

		await renderTemplateVariablesPage();

		const firstVariable = await screen.findByLabelText(
			MockTemplateVersionVariable1.name,
		);
		expect(firstVariable).toBeDefined();

		const secondVariable = await screen.findByLabelText(
			MockTemplateVersionVariable2.name,
		);
		expect(secondVariable).toBeDefined();

		// Fill the form
		const firstVariableField = await screen.findByLabelText(
			MockTemplateVersionVariable1.name,
		);
		await userEvent.clear(firstVariableField);
		await userEvent.type(firstVariableField, validFormValues.first_variable);

		const secondVariableField = await screen.findByLabelText(
			MockTemplateVersionVariable2.name,
		);
		await userEvent.clear(secondVariableField);
		await userEvent.type(secondVariableField, validFormValues.second_variable);

		// Submit the form
		const submitButton = await screen.findByText(/save/i);
		await userEvent.click(submitButton);
		// Wait for the success message
		await delay(1500);

		await screen.findByText("Template updated successfully");
	});
});
