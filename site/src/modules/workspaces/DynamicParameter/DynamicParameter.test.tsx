import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PreviewParameter } from "api/typesGenerated";
import { render } from "testHelpers/renderHelpers";
import { DynamicParameter } from "./DynamicParameter";

// Mock parameters for different form types
const createMockParameter = (
	overrides: Partial<PreviewParameter> = {},
): PreviewParameter => ({
	name: "test_param",
	display_name: "Test Parameter",
	description: "A test parameter",
	type: "string",
	mutable: true,
	default_value: "",
	icon: "",
	options: [],
	validation_error: "",
	validation_condition: "",
	validation_type_system: "",
	validation_value_type: "",
	required: false,
	legacy_variable_name: "",
	order: 1,
	form_type: "input",
	ephemeral: false,
	diagnostics: [],
	value: "",
	...overrides,
});

const mockStringParameter = createMockParameter({
	name: "string_param",
	display_name: "String Parameter",
	description: "A string input parameter",
	type: "string",
	form_type: "input",
	default_value: "default_value",
});

const mockTextareaParameter = createMockParameter({
	name: "textarea_param",
	display_name: "Textarea Parameter",
	description: "A textarea input parameter",
	type: "string",
	form_type: "textarea",
	default_value: "default\nmultiline\nvalue",
});

const mockSelectParameter = createMockParameter({
	name: "select_param",
	display_name: "Select Parameter",
	description: "A select parameter with options",
	type: "string",
	form_type: "select",
	default_value: "option1",
	options: [
		{
			name: "Option 1",
			description: "First option",
			value: "option1",
			icon: "",
		},
		{
			name: "Option 2",
			description: "Second option",
			value: "option2",
			icon: "/icon2.png",
		},
		{
			name: "Option 3",
			description: "Third option",
			value: "option3",
			icon: "",
		},
	],
});

const mockRadioParameter = createMockParameter({
	name: "radio_param",
	display_name: "Radio Parameter",
	description: "A radio button parameter",
	type: "string",
	form_type: "radio",
	default_value: "radio1",
	options: [
		{
			name: "Radio 1",
			description: "First radio option",
			value: "radio1",
			icon: "",
		},
		{
			name: "Radio 2",
			description: "Second radio option",
			value: "radio2",
			icon: "",
		},
	],
});

const mockCheckboxParameter = createMockParameter({
	name: "checkbox_param",
	display_name: "Checkbox Parameter",
	description: "A checkbox parameter",
	type: "bool",
	form_type: "checkbox",
	default_value: "true",
});

const mockSwitchParameter = createMockParameter({
	name: "switch_param",
	display_name: "Switch Parameter",
	description: "A switch parameter",
	type: "bool",
	form_type: "switch",
	default_value: "false",
});

const mockSliderParameter = createMockParameter({
	name: "slider_param",
	display_name: "Slider Parameter",
	description: "A slider parameter",
	type: "number",
	form_type: "slider",
	default_value: "50",
	validation_condition: "min=0,max=100",
});

const mockTagsParameter = createMockParameter({
	name: "tags_param",
	display_name: "Tags Parameter",
	description: "A tags parameter",
	type: "list(string)",
	form_type: "tags",
	default_value: '["tag1", "tag2"]',
});

const mockMultiSelectParameter = createMockParameter({
	name: "multiselect_param",
	display_name: "Multi-Select Parameter",
	description: "A multi-select parameter",
	type: "list(string)",
	form_type: "multiselect",
	default_value: '["option1", "option3"]',
	options: [
		{
			name: "Option 1",
			description: "First option",
			value: "option1",
			icon: "",
		},
		{
			name: "Option 2",
			description: "Second option",
			value: "option2",
			icon: "",
		},
		{
			name: "Option 3",
			description: "Third option",
			value: "option3",
			icon: "",
		},
		{
			name: "Option 4",
			description: "Fourth option",
			value: "option4",
			icon: "",
		},
	],
});

const mockErrorParameter = createMockParameter({
	name: "error_param",
	display_name: "Error Parameter",
	description: "A parameter with validation error",
	type: "string",
	form_type: "error",
	validation_error: "This parameter has a validation error",
	diagnostics: [
		{
			severity: "error",
			summary: "Validation Error",
			detail: "This parameter has a validation error",
			range: null,
		},
	],
});

const mockRequiredParameter = createMockParameter({
	name: "required_param",
	display_name: "Required Parameter",
	description: "A required parameter",
	type: "string",
	form_type: "input",
	required: true,
});

const mockImmutableParameter = createMockParameter({
	name: "immutable_param",
	display_name: "Immutable Parameter",
	description: "An immutable parameter",
	type: "string",
	form_type: "input",
	mutable: false,
	default_value: "immutable_value",
});

const mockEphemeralParameter = createMockParameter({
	name: "ephemeral_param",
	display_name: "Ephemeral Parameter",
	description: "An ephemeral parameter",
	type: "string",
	form_type: "input",
	ephemeral: true,
});

const mockParameterWithIcon = createMockParameter({
	name: "icon_param",
	display_name: "Parameter with Icon",
	description: "A parameter with an icon",
	type: "string",
	form_type: "input",
	icon: "/test-icon.png",
});

describe("DynamicParameter", () => {
	const mockOnChange = jest.fn();

	beforeEach(() => {
		jest.clearAllMocks();
	});

	describe("Input Parameter", () => {
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

		it("calls onChange when input value changes", async () => {
			render(
				<DynamicParameter
					parameter={mockStringParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			const input = screen.getByRole("textbox");
			await userEvent.type(input, "new_value");

			// Should be called for each character typed (debounced)
			await waitFor(() => {
				expect(mockOnChange).toHaveBeenCalledWith("new_value");
			});
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

		it("shows immutable indicator for immutable parameters", () => {
			render(
				<DynamicParameter
					parameter={mockImmutableParameter}
					value="immutable_value"
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText(/immutable/i)).toBeInTheDocument();
		});

		it("shows ephemeral indicator for ephemeral parameters", () => {
			render(
				<DynamicParameter
					parameter={mockEphemeralParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText(/ephemeral/i)).toBeInTheDocument();
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
			render(
				<DynamicParameter
					parameter={mockTextareaParameter}
					value="multiline\ntext\nvalue"
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Textarea Parameter")).toBeInTheDocument();
			expect(screen.getByRole("textbox")).toHaveValue("multiline\ntext\nvalue");
		});

		it("handles textarea value changes", async () => {
			render(
				<DynamicParameter
					parameter={mockTextareaParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			const textarea = screen.getByRole("textbox");
			await userEvent.type(textarea, "line1\nline2\nline3");

			await waitFor(() => {
				expect(mockOnChange).toHaveBeenCalledWith("line1\nline2\nline3");
			});
		});
	});

	describe("Select Parameter", () => {
		it("renders select parameter with options", () => {
			render(
				<DynamicParameter
					parameter={mockSelectParameter}
					value="option1"
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Select Parameter")).toBeInTheDocument();
			expect(screen.getByRole("combobox")).toBeInTheDocument();
		});

		it("displays all options when opened", async () => {
			render(
				<DynamicParameter
					parameter={mockSelectParameter}
					value="option1"
					onChange={mockOnChange}
				/>,
			);

			const select = screen.getByRole("combobox");
			await userEvent.click(select);

			expect(screen.getByText("Option 1")).toBeInTheDocument();
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

			const select = screen.getByRole("combobox");
			await userEvent.click(select);

			const option2 = screen.getByText("Option 2");
			await userEvent.click(option2);

			expect(mockOnChange).toHaveBeenCalledWith("option2");
		});

		it("displays option icons when provided", async () => {
			render(
				<DynamicParameter
					parameter={mockSelectParameter}
					value="option1"
					onChange={mockOnChange}
				/>,
			);

			const select = screen.getByRole("combobox");
			await userEvent.click(select);

			// Option 2 has an icon
			const icons = screen.getAllByRole("img");
			expect(
				icons.some((icon) => icon.getAttribute("src") === "/icon2.png"),
			).toBe(true);
		});
	});

	describe("Radio Parameter", () => {
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
			await userEvent.click(radio2);

			expect(mockOnChange).toHaveBeenCalledWith("radio2");
		});
	});

	describe("Checkbox Parameter", () => {
		it("renders checkbox parameter correctly", () => {
			render(
				<DynamicParameter
					parameter={mockCheckboxParameter}
					value="true"
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Checkbox Parameter")).toBeInTheDocument();
			expect(screen.getByRole("checkbox")).toBeChecked();
		});

		it("handles checkbox state changes", async () => {
			render(
				<DynamicParameter
					parameter={mockCheckboxParameter}
					value="true"
					onChange={mockOnChange}
				/>,
			);

			const checkbox = screen.getByRole("checkbox");
			await userEvent.click(checkbox);

			expect(mockOnChange).toHaveBeenCalledWith("false");
		});

		it("handles unchecked to checked transition", async () => {
			render(
				<DynamicParameter
					parameter={mockCheckboxParameter}
					value="false"
					onChange={mockOnChange}
				/>,
			);

			const checkbox = screen.getByRole("checkbox");
			expect(checkbox).not.toBeChecked();

			await userEvent.click(checkbox);

			expect(mockOnChange).toHaveBeenCalledWith("true");
		});
	});

	describe("Switch Parameter", () => {
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
			await userEvent.click(switchElement);

			expect(mockOnChange).toHaveBeenCalledWith("true");
		});
	});

	describe("Slider Parameter", () => {
		it("renders slider parameter correctly", () => {
			render(
				<DynamicParameter
					parameter={mockSliderParameter}
					value="50"
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByText("Slider Parameter")).toBeInTheDocument();
			expect(screen.getByRole("slider")).toHaveValue("50");
		});

		it("handles slider value changes", async () => {
			render(
				<DynamicParameter
					parameter={mockSliderParameter}
					value="50"
					onChange={mockOnChange}
				/>,
			);

			const slider = screen.getByRole("slider");
			fireEvent.change(slider, { target: { value: "75" } });

			expect(mockOnChange).toHaveBeenCalledWith("75");
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
			expect(slider).toHaveAttribute("min", "0");
			expect(slider).toHaveAttribute("max", "100");
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
			await userEvent.type(input, "newtag{enter}");

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

			// Find and click remove button for a tag
			const removeButtons = screen.getAllByRole("button", { name: /remove/i });
			await userEvent.click(removeButtons[0]);

			expect(mockOnChange).toHaveBeenCalledWith('["tag2"]');
		});
	});

	describe("Multi-Select Parameter", () => {
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
			await userEvent.click(combobox);

			const option2 = screen.getByText("Option 2");
			await userEvent.click(option2);

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

			// Find and click remove button for selected option
			const removeButtons = screen.getAllByRole("button", { name: /remove/i });
			await userEvent.click(removeButtons[0]);

			expect(mockOnChange).toHaveBeenCalledWith('["option2"]');
		});
	});

	describe("Error Parameter", () => {
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
			expect(screen.getByRole("alert")).toBeInTheDocument();
		});

		it("displays error icon", () => {
			render(
				<DynamicParameter
					parameter={mockErrorParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			// Look for error icon by checking for the error alert role
			expect(screen.getByRole("alert")).toBeInTheDocument();
		});
	});

	describe("Preset Behavior", () => {
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

		it("shows autofill indicator when autofill is true", () => {
			render(
				<DynamicParameter
					parameter={mockStringParameter}
					value="autofilled_value"
					onChange={mockOnChange}
					autofill={true}
				/>,
			);

			expect(screen.getByText(/autofilled/i)).toBeInTheDocument();
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
			const label = screen.getByText("String Parameter");

			expect(input).toHaveAccessibleName("String Parameter");
		});

		it("provides accessible descriptions", () => {
			render(
				<DynamicParameter
					parameter={mockStringParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			const input = screen.getByRole("textbox");
			expect(input).toHaveAccessibleDescription("A string input parameter");
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

		it("provides proper ARIA attributes for error states", () => {
			render(
				<DynamicParameter
					parameter={mockErrorParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			const errorAlert = screen.getByRole("alert");
			expect(errorAlert).toHaveAttribute("aria-live", "polite");
		});
	});

	describe("Debounced Input", () => {
		it("debounces input changes for text inputs", async () => {
			jest.useFakeTimers();

			render(
				<DynamicParameter
					parameter={mockStringParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			const input = screen.getByRole("textbox");

			// Type multiple characters quickly
			await userEvent.type(input, "abc");

			// Should not call onChange immediately
			expect(mockOnChange).not.toHaveBeenCalled();

			// Fast-forward time to trigger debounce
			jest.advanceTimersByTime(500);

			await waitFor(() => {
				expect(mockOnChange).toHaveBeenCalledWith("abc");
			});

			jest.useRealTimers();
		});

		it("debounces textarea changes", async () => {
			jest.useFakeTimers();

			render(
				<DynamicParameter
					parameter={mockTextareaParameter}
					value=""
					onChange={mockOnChange}
				/>,
			);

			const textarea = screen.getByRole("textbox");

			await userEvent.type(textarea, "line1\nline2");

			expect(mockOnChange).not.toHaveBeenCalled();

			jest.advanceTimersByTime(500);

			await waitFor(() => {
				expect(mockOnChange).toHaveBeenCalledWith("line1\nline2");
			});

			jest.useRealTimers();
		});
	});

	describe("Edge Cases", () => {
		it("handles empty parameter options gracefully", () => {
			const paramWithEmptyOptions = createMockParameter({
				form_type: "select",
				options: [],
			});

			render(
				<DynamicParameter
					parameter={paramWithEmptyOptions}
					value=""
					onChange={mockOnChange}
				/>,
			);

			expect(screen.getByRole("combobox")).toBeInTheDocument();
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

			// Should not crash and should render the component
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
});
