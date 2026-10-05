import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { MockChatProject } from "#/testHelpers/entities";
import themes, { DEFAULT_THEME } from "#/theme";
import { ChatProjectDialog } from "./ChatProjectDialog";

// The icon field renders external images, which read the active theme.
const Wrapper: React.FC<React.PropsWithChildren> = ({ children }) => (
	<ThemeOverride theme={themes[DEFAULT_THEME]}>{children}</ThemeOverride>
);

type DialogProps = ComponentProps<typeof ChatProjectDialog>;

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
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).toHaveBeenCalledWith(
			expect.objectContaining({ description: "d".repeat(1024) }),
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

		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).not.toHaveBeenCalled();
	});

	it("does not save without a name", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog();

		await user.type(screen.getByLabelText("Description"), "Notes");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).not.toHaveBeenCalled();
	});

	it("does not close while saving", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog({ isSubmitting: true });

		await user.keyboard("{Escape}");

		expect(props.onOpenChange).not.toHaveBeenCalled();
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
