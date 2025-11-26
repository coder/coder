import { render } from "testHelpers/renderHelpers";
import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PreviewParameter } from "api/typesGenerated";
import { DynamicParameter } from "./DynamicParameter";

const createMockParameter = (
	overrides: Partial<PreviewParameter> = {},
): PreviewParameter => ({
	name: "test_param",
	display_name: "Test Parameter",
	description: "A test parameter",
	type: "string",
	mutable: true,
	default_value: { value: "", valid: true },
	icon: "",
	options: [],
	validations: [],
	styling: {
		placeholder: "",
		disabled: false,
		label: "",
	},
	diagnostics: [],
	value: { value: "", valid: true },
	required: false,
	order: 1,
	form_type: "input",
	ephemeral: false,
	...overrides,
});

const mockStringParameter = createMockParameter({
	name: "string_param",
	display_name: "String Parameter",
	description: "A string input parameter",
	type: "string",
	form_type: "input",
	default_value: { value: "default_value", valid: true },
});

const mockTextareaParameter = createMockParameter({
	name: "textarea_param",
	display_name: "Textarea Parameter",
	description: "A textarea input parameter",
	type: "string",
	form_type: "textarea",
	default_value: { value: "default\nmultiline\nvalue", valid: true },
});

const mockTagsParameter = createMockParameter({
	name: "tags_param",
	display_name: "Tags Parameter",
	description: "A tags parameter",
	type: "list(string)",
	form_type: "tag-select",
	default_value: { value: '["tag1", "tag2"]', valid: true },
});

const mockRequiredParameter = createMockParameter({
	name: "required_param",
	display_name: "Required Parameter",
	description: "A required parameter",
	type: "string",
	form_type: "input",
	required: true,
});

describe("DynamicParameter", () => {
	const mockOnChange = jest.fn();

	beforeEach(() => {
		jest.clearAllMocks();
	});

	describe("Input Parameter", () => {
		const mockParameterWithIcon = createMockParameter({
			name: "icon_param",
			display_name: "Parameter with Icon",
			description: "A parameter with an icon",
			type: "string",
			form_type: "input",
			icon: "/test-icon.png",
		});

		it("renders string input parameter correctly", () => {
			render(
				<DynamicParameter
					parameter={mockStringParameter}
					value="test_value"
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("String Parameter")).toBeInTheDocument();
			expect(screen.getByText("A string input parameter")).toBeInTheDocument();
			expect(screen.getByRole("textbox")).toHaveValue("test_value");
		});

		it("shows required indicator for required parameters", () => {
			render(
				<DynamicParameter
					parameter={mockRequiredParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("*")).toBeInTheDocument();
		});

		it("disables input when disabled prop is true", () => {
			render(
				<DynamicParameter
					parameter={mockStringParameter}
					value=""
					onChange={mockOnChange}
					disabled={true}
				/>,
			);

			expect(screen.getByRole("textbox")).toBeDisabled();
		});

		it("displays parameter icon when provided", () => {
			render(
				<DynamicParameter
					parameter={mockParameterWithIcon}
					value=""
					onChange={mockOnChange}
				/>,
			);

			const icon = screen.getByRole("img");
			expect(icon).toHaveAttribute("src", "/test-icon.png");
		});
	});

	describe("Textarea Parameter", () => {
		it("renders textarea parameter correctly", () => {
			const testValue = "multiline\ntext\nvalue";
			render(
				<DynamicParameter
					parameter={mockTextareaParameter}
					value={testValue}
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Textarea Parameter")).toBeInTheDocument();
			expect(screen.getByRole("textbox")).toHaveValue(testValue);
		});
	});

	describe("dropdown parameter", () => {
		const mockSelectParameter = createMockParameter({
			name: "select_param",
			display_name: "Select Parameter",
			description: "A select parameter with options",
			type: "string",
			form_type: "dropdown",
			default_value: { value: "option1", valid: true },
			options: [
				{
					name: "Option 1",
					description: "First option",
					value: { value: "option1", valid: true },
					icon: "",
				},
				{
					name: "Option 2",
					description: "Second option",
					value: { value: "option2", valid: true },
					icon: "/icon2.png",
				},
				{
					name: "Option 3",
					description: "Third option",
					value: { value: "option3", valid: true },
					icon: "",
				},
			],
		});

		it("displays all options when opened", async () => {
			render(
				<DynamicParameter
					parameter={mockSelectParameter}
					value="option1"
					onChange={mockOnChange}
				/>,
			);

			const select = screen.getByRole("button");
			await waitFor(async () => {
				await userEvent.click(select);
			});

			// Option 1 exists in the trigger and the dropdown
			expect(screen.getAllByText("Option 1")).toHaveLength(2);
			expect(screen.getByText("Option 2")).toBeInTheDocument();
			expect(screen.getByText("Option 3")).toBeInTheDocument();
		});

		it("calls onChange when option is selected", async () => {
			render(
				<DynamicParameter
					parameter={mockSelectParameter}
					value="option1"
					onChange={mockOnChange}
				/>,
			);

			const select = screen.getByRole("button");
			await waitFor(async () => {
				await userEvent.click(select);
			});

			const option2 = screen.getByText("Option 2");
			await waitFor(async () => {
				await userEvent.click(option2);
			});

			expect(mockOnChange).toHaveBeenCalledWith("option2");
		});
	});

	describe("Radio Parameter", () => {
		const mockRadioParameter = createMockParameter({
			name: "radio_param",
			display_name: "Radio Parameter",
			description: "A radio button parameter",
			type: "string",
			form_type: "radio",
			default_value: { value: "radio1", valid: true },
			options: [
				{
					name: "Radio 1",
					description: "First radio option",
					value: { value: "radio1", valid: true },
					icon: "",
				},
				{
					name: "Radio 2",
					description: "Second radio option",
					value: { value: "radio2", valid: true },
					icon: "",
				},
			],
		});

		it("renders radio parameter with options", () => {
			render(
				<DynamicParameter
					parameter={mockRadioParameter}
					value="radio1"
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Radio Parameter")).toBeInTheDocument();
			expect(screen.getByRole("radiogroup")).toBeInTheDocument();
			expect(screen.getByRole("radio", { name: /radio 1/i })).toBeChecked();
			expect(screen.getByRole("radio", { name: /radio 2/i })).not.toBeChecked();
		});

		it("calls onChange when radio option is selected", async () => {
			render(
				<DynamicParameter
					parameter={mockRadioParameter}
					value="radio1"
					onChange={mockOnChange}
				/>,
			);

			const radio2 = screen.getByRole("radio", { name: /radio 2/i });
			await waitFor(async () => {
				await userEvent.click(radio2);
			});

			expect(mockOnChange).toHaveBeenCalledWith("radio2");
		});
	});

	describe("Checkbox Parameter", () => {
		const mockCheckboxParameter = createMockParameter({
			name: "checkbox_param",
			display_name: "Checkbox Parameter",
			description: "A checkbox parameter",
			type: "bool",
			form_type: "checkbox",
			default_value: { value: "true", valid: true },
		});

		it("Renders checkbox parameter correctly and handles unchecked to checked transition", async () => {
			render(
				<DynamicParameter
					parameter={mockCheckboxParameter}
					value="false"
					onChange={mockOnChange}
				/>,
			);
			expect(screen.getByText("Checkbox Parameter")).toBeInTheDocument();

			const checkbox = screen.getByRole("checkbox");
			expect(checkbox).not.toBeChecked();

			await waitFor(async () => {
				await userEvent.click(checkbox);
			});

			expect(mockOnChange).toHaveBeenCalledWith("true");
		});
	});

	describe("Switch Parameter", () => {
		const mockSwitchParameter = createMockParameter({
			name: "switch_param",
			display_name: "Switch Parameter",
			description: "A switch parameter",
			type: "bool",
			form_type: "switch",
			default_value: { value: "false", valid: true },
		});

		it("renders switch parameter correctly", () => {
			render(
				<DynamicParameter
					parameter={mockSwitchParameter}
					value="false"
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Switch Parameter")).toBeInTheDocument();
			expect(screen.getByRole("switch")).not.toBeChecked();
		});

		it("handles switch state changes", async () => {
			render(
				<DynamicParameter
					parameter={mockSwitchParameter}
					value="false"
					onChange={mockOnChange}
				/>,
			);

			const switchElement = screen.getByRole("switch");
			await waitFor(async () => {
				await userEvent.click(switchElement);
			});

			expect(mockOnChange).toHaveBeenCalledWith("true");
		});
	});

	describe("Slider Parameter", () => {
		const mockSliderParameter = createMockParameter({
			name: "slider_param",
			display_name: "Slider Parameter",
			description: "A slider parameter",
			type: "number",
			form_type: "slider",
			default_value: { value: "50", valid: true },
			validations: [
				{
					validation_min: 0,
					validation_max: 100,
					validation_error: "Value must be between 0 and 100",
					validation_regex: null,
					validation_monotonic: null,
				},
			],
		});

		it("renders slider parameter correctly", () => {
			render(
				<DynamicParameter
					parameter={mockSliderParameter}
					value="50"
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Slider Parameter")).toBeInTheDocument();
			const slider = screen.getByRole("slider");
			expect(slider).toHaveAttribute("aria-valuenow", "50");
		});

		it("respects min/max constraints from validation_condition", () => {
			render(
				<DynamicParameter
					parameter={mockSliderParameter}
					value="50"
					onChange={mockOnChange}
				/>,
			);

			const slider = screen.getByRole("slider");
			expect(slider).toHaveAttribute("aria-valuemin", "0");
			expect(slider).toHaveAttribute("aria-valuemax", "100");
		});
	});

	describe("Tags Parameter", () => {
		it("renders tags parameter correctly", () => {
			render(
				<DynamicParameter
					parameter={mockTagsParameter}
					value='["tag1", "tag2", "tag3"]'
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Tags Parameter")).toBeInTheDocument();
			expect(screen.getByRole("textbox")).toBeInTheDocument();
		});

		it("handles tag additions", async () => {
			render(
				<DynamicParameter
					parameter={mockTagsParameter}
					value='["tag1"]'
					onChange={mockOnChange}
				/>,
			);

			const input = screen.getByRole("textbox");
			await waitFor(async () => {
				await userEvent.type(input, "newtag,");
			});

			await waitFor(() => {
				expect(mockOnChange).toHaveBeenCalledWith('["tag1","newtag"]');
			});
		});

		it("handles tag removals", async () => {
			render(
				<DynamicParameter
					parameter={mockTagsParameter}
					value='["tag1", "tag2"]'
					onChange={mockOnChange}
				/>,
			);

			const deleteButtons = screen.getAllByTestId("CancelIcon");
			await waitFor(async () => {
				await userEvent.click(deleteButtons[0]);
			});

			expect(mockOnChange).toHaveBeenCalledWith('["tag2"]');
		});
	});

	describe("Multi-Select Parameter", () => {
		const mockMultiSelectParameter = createMockParameter({
			name: "multiselect_param",
			display_name: "Multi-Select Parameter",
			description: "A multi-select parameter",
			type: "list(string)",
			form_type: "multi-select",
			default_value: { value: '["option1", "option3"]', valid: true },
			options: [
				{
					name: "Option 1",
					description: "First option",
					value: { value: "option1", valid: true },
					icon: "",
				},
				{
					name: "Option 2",
					description: "Second option",
					value: { value: "option2", valid: true },
					icon: "",
				},
				{
					name: "Option 3",
					description: "Third option",
					value: { value: "option3", valid: true },
					icon: "",
				},
				{
					name: "Option 4",
					description: "Fourth option",
					value: { value: "option4", valid: true },
					icon: "",
				},
			],
		});

		it("renders multi-select parameter correctly", () => {
			render(
				<DynamicParameter
					parameter={mockMultiSelectParameter}
					value='["option1", "option3"]'
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Multi-Select Parameter")).toBeInTheDocument();
			expect(screen.getByRole("combobox")).toBeInTheDocument();
		});

		it("displays selected options", () => {
			render(
				<DynamicParameter
					parameter={mockMultiSelectParameter}
					value='["option1", "option3"]'
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Option 1")).toBeInTheDocument();
			expect(screen.getByText("Option 3")).toBeInTheDocument();
		});

		it("handles option selection", async () => {
			render(
				<DynamicParameter
					parameter={mockMultiSelectParameter}
					value='["option1"]'
					onChange={mockOnChange}
				/>,
			);

			const combobox = screen.getByRole("combobox");
			await waitFor(async () => {
				await userEvent.click(combobox);
			});

			const option2 = screen.getByText("Option 2");
			await waitFor(async () => {
				await userEvent.click(option2);
			});

			expect(mockOnChange).toHaveBeenCalledWith('["option1","option2"]');
		});

		it("handles option deselection", async () => {
			render(
				<DynamicParameter
					parameter={mockMultiSelectParameter}
					value='["option1", "option2"]'
					onChange={mockOnChange}
				/>,
			);

			const removeButtons = screen.getAllByTestId("clear-option-button");
			await waitFor(async () => {
				await userEvent.click(removeButtons[0]);
			});

			expect(mockOnChange).toHaveBeenCalledWith('["option2"]');
		});
	});

	describe("Error Parameter", () => {
		const mockErrorParameter = createMockParameter({
			name: "error_param",
			display_name: "Error Parameter",
			description: "A parameter with validation error",
			type: "string",
			form_type: "error",
			diagnostics: [
				{
					severity: "error",
					summary: "Validation Error",
					detail: "This parameter has a validation error",
					extra: {
						code: "validation_error",
					},
				},
			],
		});

		it("renders error parameter with validation message", () => {
			render(
				<DynamicParameter
					parameter={mockErrorParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Error Parameter")).toBeInTheDocument();
			expect(
				screen.getByText("This parameter has a validation error"),
			).toBeInTheDocument();
		});
	});

	describe("Parameter Badges", () => {
		const mockEphemeralParameter = createMockParameter({
			name: "ephemeral_param",
			display_name: "Ephemeral Parameter",
			description: "An ephemeral parameter",
			type: "string",
			form_type: "input",
			ephemeral: true,
		});

		const mockImmutableParameter = createMockParameter({
			name: "immutable_param",
			display_name: "Immutable Parameter",
			description: "An immutable parameter",
			type: "string",
			form_type: "input",
			mutable: false,
			default_value: { value: "immutable_value", valid: true },
		});

		it("shows immutable indicator for immutable parameters", () => {
			render(
				<DynamicParameter
					parameter={mockImmutableParameter}
					value="immutable_value"
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Immutable")).toBeInTheDocument();
		});

		it("shows autofill indicator when autofill is true", () => {
			render(
				<DynamicParameter
					parameter={mockStringParameter}
					value="autofilled_value"
					onChange={mockOnChange}
					autofill={true}
				/>,
			);

			expect(screen.getByText(/URL Autofill/i)).toBeInTheDocument();
		});

		it("shows ephemeral indicator for ephemeral parameters", () => {
			render(
				<DynamicParameter
					parameter={mockEphemeralParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Ephemeral")).toBeInTheDocument();
		});

		it("shows preset indicator when isPreset is true", () => {
			render(
				<DynamicParameter
					parameter={mockStringParameter}
					value="preset_value"
					onChange={mockOnChange}
					isPreset={true}
				/>,
			);

			expect(screen.getByText(/preset/i)).toBeInTheDocument();
		});
	});

	describe("Accessibility", () => {
		it("associates labels with form controls", () => {
			render(
				<DynamicParameter
					parameter={mockStringParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			const input = screen.getByRole("textbox");

			expect(input).toHaveAccessibleName("String Parameter");
		});

		it("marks required fields appropriately", () => {
			render(
				<DynamicParameter
					parameter={mockRequiredParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			const input = screen.getByRole("textbox");
			expect(input).toBeRequired();
		});
	});

	describe("Edge Cases", () => {
		it("handles empty parameter options gracefully", () => {
			const paramWithEmptyOptions = createMockParameter({
				form_type: "dropdown",
				options: [],
			});

			render(
				<DynamicParameter
					parameter={paramWithEmptyOptions}
					value=""
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByRole("button")).toBeInTheDocument();
		});

		it("handles null/undefined values", () => {
			render(
				<DynamicParameter
					parameter={mockStringParameter}
					value={undefined}
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByRole("textbox")).toHaveValue("");
		});

		it("handles invalid JSON in list parameters", () => {
			render(
				<DynamicParameter
					parameter={mockTagsParameter}
					value="invalid json"
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Tags Parameter")).toBeInTheDocument();
		});

		it("handles parameters with very long descriptions", () => {
			const longDescriptionParam = createMockParameter({
				description: "A".repeat(1000),
			});

			render(
				<DynamicParameter
					parameter={longDescriptionParam}
					value=""
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("A".repeat(1000))).toBeInTheDocument();
		});

		it("handles parameters with special characters in names", () => {
			const specialCharParam = createMockParameter({
				name: "param-with_special.chars",
				display_name: "Param with Special Characters!@#$%",
			});

			render(
				<DynamicParameter
					parameter={specialCharParam}
					value=""
					onChange={mockOnChange}
				/>,
			);

			expect(
				screen.getByText("Param with Special Characters!@#$%"),
			).toBeInTheDocument();
		});
	});

	describe("Number Input Parameter", () => {
		const mockNumberInputParameter = createMockParameter({
			name: "number_input_param",
			display_name: "Number Input Parameter",
			description: "A numeric input parameter with min/max validations",
			type: "number",
			form_type: "input",
			default_value: { value: "5", valid: true },
			validations: [
				{
					validation_min: 1,
					validation_max: 10,
					validation_error: "Value must be between 1 and 10",
					validation_regex: null,
					validation_monotonic: null,
				},
			],
		});

		it("renders number input with correct min/max attributes", () => {
			render(
				<DynamicParameter
					parameter={mockNumberInputParameter}
					value="5"
					onChange={mockOnChange}
				/>,
			);

			const input = screen.getByRole("spinbutton");
			expect(input).toHaveAttribute("min", "1");
			expect(input).toHaveAttribute("max", "10");
		});

		it("calls onChange when numeric value changes (debounced)", () => {
			jest.useFakeTimers();
			render(
				<DynamicParameter
					parameter={mockNumberInputParameter}
					value="5"
					onChange={mockOnChange}
				/>,
			);

			const input = screen.getByRole("spinbutton");
			fireEvent.change(input, { target: { value: "7" } });

			act(() => {
				jest.runAllTimers();
			});

			expect(mockOnChange).toHaveBeenCalledWith("7");
			jest.useRealTimers();
		});
	});

	describe("Masked Input Parameter", () => {
		const mockMaskedInputParameter = createMockParameter({
			name: "masked_input_param",
			display_name: "Masked Input Parameter",
			type: "string",
			form_type: "input",
			styling: {
				placeholder: "********",
				disabled: false,
				label: "",
				mask_input: true,
			},
		});

		it("renders a password field by default and toggles visibility on mouse events", async () => {
			render(
				<DynamicParameter
					parameter={mockMaskedInputParameter}
					value="secret123"
					onChange={mockOnChange}
				/>,
			);

			const input = screen.getByLabelText("Masked Input Parameter");
			expect(input).toHaveAttribute("type", "password");

			const toggleButton = screen.getByRole("button");
			fireEvent.mouseDown(toggleButton);
			expect(input).toHaveAttribute("type", "text");

			fireEvent.mouseUp(toggleButton);
			expect(input).toHaveAttribute("type", "password");
		});
	});

	describe("Parameter Diagnostics", () => {
		const mockWarningParameter = createMockParameter({
			name: "warning_param",
			display_name: "Warning Parameter",
			description: "Parameter with a warning diagnostic",
			form_type: "input",
			diagnostics: [
				{
					severity: "warning",
					summary: "This is a warning",
					detail: "Something might be wrong",
					extra: { code: "warning" },
				},
			],
		});

		it("renders warning diagnostics for non-error parameters", () => {
			render(
				<DynamicParameter
					parameter={mockWarningParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("This is a warning")).toBeInTheDocument();
			expect(screen.getByText("Something might be wrong")).toBeInTheDocument();
		});
	});
});
