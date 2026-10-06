import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { flushSync } from "react-dom";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import {
	MockChatProject,
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import themes, { DEFAULT_THEME } from "#/theme";
import { ChatProjectDialog } from "./ChatProjectDialog";

// The icon field renders external images, which read the active theme.
const Wrapper: React.FC<React.PropsWithChildren> = ({ children }) => (
	<ThemeOverride theme={themes[DEFAULT_THEME]}>{children}</ThemeOverride>
);

type DialogProps = React.ComponentProps<typeof ChatProjectDialog>;

const defaultProps = {
	open: true,
	organizations: [MockDefaultOrganization, MockOrganization2],
	initialOrganizationId: MockDefaultOrganization.id,
	onOpenChange: vi.fn(),
	isSubmitting: false,
	error: undefined,
	onSubmit: vi.fn(),
} satisfies DialogProps;

const renderDialog = (props: Partial<DialogProps> = {}) => {
	const allProps: DialogProps = {
		...defaultProps,
		onOpenChange: vi.fn(),
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
	it("does not submit an unchanged project when Save is activated before async validation resolves", async () => {
		const onSubmit = vi.fn();
		const container = document.createElement("div");
		document.body.appendChild(container);
		const root = createRoot(container);
		try {
			// Render and click synchronously so the click lands before the
			// mount-time validation promise settles.
			flushSync(() => {
				root.render(
					<Wrapper>
						<ChatProjectDialog
							{...defaultProps}
							project={MockChatProject}
							onSubmit={onSubmit}
						/>
					</Wrapper>,
				);
			});
			screen.getByRole("button", { name: "Save" }).click();

			// Validation and Formik's submit only chain promises, so they
			// settle before the next macrotask.
			await new Promise((resolve) => setTimeout(resolve, 0));
			expect(onSubmit).not.toHaveBeenCalled();
		} finally {
			flushSync(() => {
				root.unmount();
			});
			container.remove();
		}
	});

	it("submits trimmed values and the organization selected with the keyboard after a failed attempt", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const { rerender } = render(
			<ChatProjectDialog {...defaultProps} onSubmit={onSubmit} />,
			{ wrapper: Wrapper },
		);

		await user.type(
			screen.getByRole("textbox", { name: /Project name/ }),
			"  My project  ",
		);
		await user.tab();
		await user.tab();
		await user.keyboard("{Enter}");
		await user.type(
			screen.getByRole("combobox"),
			MockOrganization2.display_name,
		);
		await user.keyboard("{ArrowDown}{Enter}");
		await user.type(
			screen.getByRole("textbox", { name: "Description" }),
			"  Project description  ",
		);
		await user.type(
			screen.getByRole("textbox", { name: "Icon" }),
			"  /emojis/1f4c1.png  ",
		);
		await user.click(screen.getByRole("button", { name: "Create project" }));

		const values = {
			name: "My project",
			description: "Project description",
			icon: "/emojis/1f4c1.png",
			organizationId: MockOrganization2.id,
		};
		await waitFor(() => expect(onSubmit).toHaveBeenCalledWith(values));

		rerender(
			<ChatProjectDialog
				{...defaultProps}
				organizations={[
					{ ...MockDefaultOrganization },
					{ ...MockOrganization2 },
				]}
				error={new Error("Please try again.")}
				onSubmit={onSubmit}
			/>,
		);
		await user.click(screen.getByRole("button", { name: "Create project" }));
		await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(2));
		expect(onSubmit).toHaveBeenLastCalledWith(values);
	});

	it.each([
		{ organizations: [], initialOrganizationId: undefined },
		{
			organizations: [MockDefaultOrganization],
			initialOrganizationId: undefined,
		},
		{
			organizations: [MockDefaultOrganization],
			initialOrganizationId: MockOrganization2.id,
		},
	])("blocks creation without an available selection: %j", async (props) => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		render(
			<ChatProjectDialog {...defaultProps} {...props} onSubmit={onSubmit} />,
			{ wrapper: Wrapper },
		);
		await user.type(
			screen.getByRole("textbox", { name: /Project name/ }),
			"My project{Enter}",
		);
		await user.click(screen.getByRole("button", { name: "Create project" }));
		expect(onSubmit).not.toHaveBeenCalled();
	});

	it("requires a new selection when the selected organization becomes unavailable", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const { rerender } = render(
			<ChatProjectDialog {...defaultProps} onSubmit={onSubmit} />,
			{ wrapper: Wrapper },
		);
		await user.type(
			screen.getByRole("textbox", { name: /Project name/ }),
			"My project",
		);
		rerender(
			<ChatProjectDialog
				{...defaultProps}
				organizations={[MockOrganization2]}
				onSubmit={onSubmit}
			/>,
		);
		await user.type(
			screen.getByRole("textbox", { name: /Project name/ }),
			"{Enter}",
		);
		await user.click(screen.getByRole("button", { name: "Create project" }));
		expect(onSubmit).not.toHaveBeenCalled();
		await user.click(screen.getByRole("button", { name: /Organization/ }));
		await user.click(
			screen.getByRole("option", {
				name: new RegExp(MockOrganization2.display_name),
			}),
		);
		await user.click(screen.getByRole("button", { name: "Create project" }));
		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith({
				name: "My project",
				description: "",
				icon: "",
				organizationId: MockOrganization2.id,
			}),
		);
	});

	it("resets the form when reopened", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const { rerender } = render(
			<ChatProjectDialog {...defaultProps} onSubmit={onSubmit} />,
			{ wrapper: Wrapper },
		);
		await user.type(
			screen.getByRole("textbox", { name: /Project name/ }),
			"Abandoned project",
		);
		await user.click(screen.getByRole("button", { name: /Organization/ }));
		await user.click(
			screen.getByRole("option", {
				name: new RegExp(MockOrganization2.display_name),
			}),
		);
		rerender(
			<ChatProjectDialog {...defaultProps} open={false} onSubmit={onSubmit} />,
		);
		rerender(<ChatProjectDialog {...defaultProps} onSubmit={onSubmit} />);
		await user.type(
			screen.getByRole("textbox", { name: /Project name/ }),
			"New project",
		);
		await user.click(screen.getByRole("button", { name: "Create project" }));
		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith({
				name: "New project",
				description: "",
				icon: "",
				organizationId: MockDefaultOrganization.id,
			}),
		);
	});

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

		await user.type(screen.getByLabelText(/Project name/), "  Launch  ");
		await user.type(screen.getByLabelText("Description"), " Notes ");
		await user.click(screen.getByRole("button", { name: "Create project" }));

		expect(props.onSubmit).toHaveBeenCalledWith({
			name: "Launch",
			description: "Notes",
			icon: "",
			organizationId: MockDefaultOrganization.id,
		});
	});

	it("counts an emoji as one character toward the name limit", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog();

		await user.click(screen.getByLabelText(/Project name/));
		await user.paste("🚀".repeat(64));
		expect(screen.getByLabelText(/Project name/)).toHaveAttribute(
			"aria-invalid",
			"false",
		);
		await user.click(screen.getByRole("button", { name: "Create project" }));

		expect(props.onSubmit).toHaveBeenCalledWith(
			expect.objectContaining({ name: "🚀".repeat(64) }),
		);

		await user.click(screen.getByLabelText(/Project name/));
		await user.paste("🚀");
		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: "Create project" }),
			).toBeDisabled(),
		);
	});

	it("measures the length limit after trimming", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog();

		await user.type(screen.getByLabelText(/Project name/), "Launch");
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
		await user.click(screen.getByRole("button", { name: "Create project" }));

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

		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSubmit).not.toHaveBeenCalled();
	});

	it("does not save a name made only of spaces", async () => {
		const user = userEvent.setup();
		const { props } = renderDialog();

		await user.type(screen.getByLabelText(/Project name/), "   ");
		await user.click(screen.getByRole("button", { name: "Create project" }));

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
		await user.click(screen.getByRole("button", { name: "Create project" }));

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
