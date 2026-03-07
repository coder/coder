import { render, screen } from "@testing-library/react";
import { TextareaField } from "./TextareaField";

describe("TextareaField", () => {
	it("renders a textarea element", () => {
		render(<TextareaField />);
		expect(screen.getByRole("textbox")).toBeInTheDocument();
	});

	it("renders a label associated with the textarea", () => {
		render(<TextareaField id="desc" label="Description" />);
		const label = screen.getByText("Description");
		expect(label.tagName).toBe("LABEL");
		expect(label).toHaveAttribute("for", "desc");
		expect(screen.getByLabelText("Description")).toBeInTheDocument();
	});

	it("does not render a label element when label is omitted", () => {
		render(<TextareaField />);
		expect(screen.queryByRole("label")).not.toBeInTheDocument();
	});

	it("renders helper text below the textarea", () => {
		render(<TextareaField helperText="Markdown is supported." />);
		expect(screen.getByText("Markdown is supported.")).toBeInTheDocument();
	});

	it("applies error styling to label and helper text when error is true", () => {
		render(
			<TextareaField id="msg" label="Message" error helperText="Required." />,
		);
		expect(screen.getByText("Message")).toHaveClass("text-content-destructive");
		expect(screen.getByText("Required.")).toHaveClass(
			"text-content-destructive",
		);
	});

	it("does not apply error styling when error is false", () => {
		render(
			<TextareaField
				id="msg"
				label="Message"
				error={false}
				helperText="Optional note."
			/>,
		);
		expect(screen.getByText("Message")).not.toHaveClass(
			"text-content-destructive",
		);
		expect(screen.getByText("Optional note.")).not.toHaveClass(
			"text-content-destructive",
		);
	});

	it("sets aria-invalid and aria-errormessage when error is true", () => {
		render(
			<TextareaField id="msg" error helperText="This field is required." />,
		);
		const textarea = screen.getByRole("textbox");
		expect(textarea).toHaveAttribute("aria-invalid", "true");
		expect(textarea).toHaveAttribute("aria-errormessage", "msg-helper-text");
		expect(textarea).not.toHaveAttribute("aria-describedby");
	});

	it("sets aria-describedby to the helper text when there is no error", () => {
		render(
			<TextareaField
				id="msg"
				error={false}
				helperText="Markdown is supported."
			/>,
		);
		const textarea = screen.getByRole("textbox");
		expect(textarea).not.toHaveAttribute("aria-invalid");
		expect(textarea).toHaveAttribute("aria-describedby", "msg-helper-text");
		expect(textarea).not.toHaveAttribute("aria-errormessage");
	});

	it("gives the helper text element a matching id", () => {
		render(<TextareaField id="msg" helperText="Some helper text." />);
		expect(screen.getByText("Some helper text.")).toHaveAttribute(
			"id",
			"msg-helper-text",
		);
	});

	it("passes through standard textarea props", () => {
		render(
			<TextareaField
				name="licenseKey"
				placeholder="Enter your license..."
				rows={3}
				defaultValue="hello"
			/>,
		);
		const textarea = screen.getByRole("textbox");
		expect(textarea).toHaveAttribute("name", "licenseKey");
		expect(textarea).toHaveAttribute("placeholder", "Enter your license...");
		expect(textarea).toHaveAttribute("rows", "3");
		expect(textarea).toHaveValue("hello");
	});

	it("disables the textarea when disabled is true", () => {
		render(<TextareaField disabled />);
		expect(screen.getByRole("textbox")).toBeDisabled();
	});
});
