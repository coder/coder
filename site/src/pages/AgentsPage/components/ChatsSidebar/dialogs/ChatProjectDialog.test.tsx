import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { MockChatProject, mockApiError } from "#/testHelpers/entities";
import themes, { DEFAULT_THEME } from "#/theme";
import { ChatProjectDialog } from "./ChatProjectDialog";

// The icon field renders external images, which read the active theme.
const Wrapper: React.FC<React.PropsWithChildren> = ({ children }) => (
	<ThemeOverride theme={themes[DEFAULT_THEME]}>{children}</ThemeOverride>
);

type DialogProps = React.ComponentProps<typeof ChatProjectDialog>;

const renderDialog = (props: Partial<DialogProps> = {}) => {
	const allProps: DialogProps = {
		open: true,
		onOpenChange: vi.fn(),
		isSubmitting: false,
		error: undefined,
		onSubmit: vi.fn(),
		...props,
	};
	const view = render(<ChatProjectDialog {...allProps} />, {
		wrapper: Wrapper,
	});
	return {
		...view,
		props: allProps,
		rerenderWith: (next: Partial<DialogProps>) =>
			view.rerender(<ChatProjectDialog {...allProps} {...next} />),
	};
};

// A disabled Save button does not submit. Dispatching submit checks that the
// handler itself drops an unchanged edit.
const submitForm = (save: HTMLElement) => {
	if (!(save instanceof HTMLButtonElement)) {
		throw new Error("Save is not a button");
	}
	save.form?.dispatchEvent(
		new Event("submit", { bubbles: true, cancelable: true }),
	);
};

describe("ChatProjectDialog", () => {
	it("fills the form from the project passed when the dialog opens and submits it", async () => {
		const user = userEvent.setup();
		const project = { ...MockChatProject, icon: "/emojis/1f4c1.png" };
		// Callers mount the dialog closed and pass the project when opening it.
		const { props, rerenderWith } = renderDialog({ open: false });
		rerenderWith({ open: true, project });

		await user.clear(screen.getByLabelText(/Name/));
		await user.type(screen.getByLabelText(/Name/), "Renamed");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).toHaveBeenCalledWith({
			name: "Renamed",
			description: project.description,
			icon: project.icon,
		});
	});

	it("trims the values it submits when creating a project", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog();

		await user.type(screen.getByLabelText(/Name/), "  Launch  ");
		await user.type(screen.getByLabelText("Description"), " Notes ");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).toHaveBeenCalledWith({
			name: "Launch",
			description: "Notes",
			icon: "",
		});
	});

	it("counts an emoji as one character toward the name limit", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog();

		await user.click(screen.getByLabelText(/Name/));
		await user.paste("🚀".repeat(64));
		expect(screen.getByLabelText(/Name/)).toHaveAttribute(
			"aria-invalid",
			"false",
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).toHaveBeenCalledWith(
			expect.objectContaining({ name: "🚀".repeat(64) }),
		);

		await user.click(screen.getByLabelText(/Name/));
		await user.paste("🚀");
		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Save" })).toBeDisabled(),
		);
	});

	it("measures the length limit after trimming", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog();

		await user.type(screen.getByLabelText(/Name/), "Launch");
		await user.click(screen.getByLabelText("Description"));
		await user.paste(`${"d".repeat(1024)} `);
		expect(screen.getByLabelText("Description")).toHaveAttribute(
			"aria-invalid",
			"false",
		);
		await user.click(screen.getByLabelText("Icon"));
		await user.paste(`${"i".repeat(256)} `);
		expect(screen.getByLabelText("Icon")).toHaveAttribute(
			"aria-invalid",
			"false",
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).toHaveBeenCalledWith(
			expect.objectContaining({
				description: "d".repeat(1024),
				icon: "i".repeat(256),
			}),
		);
	});

	it("saves an edit that changes only the icon", async () => {
		const user = userEvent.setup();
		const project = { ...MockChatProject, icon: "" };
		const { props } = renderDialog({ project });

		await user.type(screen.getByLabelText("Icon"), "/emojis/1f680.png");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).toHaveBeenCalledWith({
			name: project.name,
			description: project.description,
			icon: "/emojis/1f680.png",
		});
	});

	it("does not save an edit when only stored whitespace differs", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog({
			project: { ...MockChatProject, description: "notes\n" },
		});

		submitForm(screen.getByRole("button", { name: "Save" }));
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).not.toHaveBeenCalled();
	});

	it("does not save a name made only of spaces", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog();

		await user.type(screen.getByLabelText(/Name/), "   ");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).not.toHaveBeenCalled();
	});

	it("does not save an edit that only adds whitespace", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog({ project: MockChatProject });

		await user.type(screen.getByLabelText(/Name/), " ");

		expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
		expect(props.onSubmit).not.toHaveBeenCalled();
	});

	it("does not save an edit that changes nothing", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog({ project: MockChatProject });
		const save = screen.getByRole("button", { name: "Save" });

		expect(save).toBeDisabled();
		submitForm(save);
		await user.click(save);
		await user.click(screen.getByLabelText(/Name/));
		await user.keyboard("{Enter}");

		expect(props.onSubmit).not.toHaveBeenCalled();
	});

	it("does not save without a name", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog();

		await user.type(screen.getByLabelText("Description"), "Notes");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).not.toHaveBeenCalled();
	});

	it("explains that a name is required once the field is left empty", async () => {
		const user = userEvent.setup();
		renderDialog();

		await user.click(screen.getByLabelText(/Name/));
		await user.tab();

		expect(screen.getByText("Name is required.")).toBeVisible();
	});

	it("shows a save error that is not tied to a field", () => {
		renderDialog({
			error: mockApiError({
				message:
					"You can have at most 100 chat projects. Delete a project to create another.",
			}),
		});

		expect(screen.getByText(/at most 100 chat projects/)).toBeVisible();
		expect(screen.queryByText("Response data")).not.toBeInTheDocument();
	});

	it("shows an API field error on the field", async () => {
		const user = userEvent.setup();
		const message = "Name must be at most 64 characters.";
		const { props, rerenderWith } = renderDialog();

		await user.type(screen.getByLabelText(/Name/), "Launch");
		await user.click(screen.getByRole("button", { name: "Save" }));
		expect(props.onSubmit).toHaveBeenCalled();

		rerenderWith({
			error: mockApiError({
				message,
				validations: [{ field: "name", detail: message }],
			}),
		});

		expect(screen.getAllByText(message)).toHaveLength(1);
		expect(screen.getByLabelText(/Name/)).toHaveAttribute(
			"aria-invalid",
			"true",
		);
	});

	it("does not close while saving", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog({ isSubmitting: true });

		await user.keyboard("{Escape}");

		expect(props.onOpenChange).not.toHaveBeenCalled();
	});

	it("submits once when Save is double-clicked before the save settles", async () => {
		const user = userEvent.setup();
		// The caller's isSubmitting has not caught up yet, so only the dialog's
		// own pending submit can stop the second click.
		const onSubmit = vi.fn(() => new Promise<void>(() => {}));
		renderDialog({ onSubmit });

		await user.type(screen.getByLabelText(/Name/), "Launch");
		await user.dblClick(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).toHaveBeenCalledTimes(1);
	});

	it("can save again after a save fails", async () => {
		const user = userEvent.setup();
		const onSubmit = vi
			.fn<DialogProps["onSubmit"]>()
			.mockRejectedValueOnce(new Error("Save failed"))
			.mockResolvedValueOnce(undefined);
		renderDialog({ onSubmit });

		await user.type(screen.getByLabelText(/Name/), "Launch");
		const save = screen.getByRole("button", { name: "Save" });
		await user.click(save);
		// Wait for the failed submit to release the form before retrying.
		await waitFor(() => expect(save).toBeEnabled());
		await user.click(save);

		await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(2));
	});

	it("discards unsaved edits when it is closed and reopened", async () => {
		const user = userEvent.setup();
		const { props, rerenderWith } = renderDialog({ project: MockChatProject });

		await user.type(screen.getByLabelText(/Name/), "xyz");
		rerenderWith({ open: false });
		rerenderWith({ open: true });
		await user.type(screen.getByLabelText("Description"), "!");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).toHaveBeenCalledWith({
			name: MockChatProject.name,
			description: `${MockChatProject.description}!`,
			icon: MockChatProject.icon,
		});
	});
});
