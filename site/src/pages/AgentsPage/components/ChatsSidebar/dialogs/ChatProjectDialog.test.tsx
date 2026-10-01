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

const defaultProps = {
	open: true,
	organizations: [MockDefaultOrganization, MockOrganization2],
	initialOrganizationId: MockDefaultOrganization.id,
	onOpenChange: vi.fn(),
	isSubmitting: false,
	error: undefined,
	onSubmit: vi.fn(),
} satisfies React.ComponentProps<typeof ChatProjectDialog>;

describe("ChatProjectDialog", () => {
	it("submits an unchanged project when Save is activated before async validation resolves", async () => {
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

			await waitFor(() =>
				expect(onSubmit).toHaveBeenCalledWith({
					name: MockChatProject.name,
					description: MockChatProject.description,
					icon: MockChatProject.icon,
				}),
			);
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

	it("submits the selected project's name, description, and icon", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const project = { ...MockChatProject, icon: "/emojis/1f4c1.png" };
		const { rerender } = render(
			<ChatProjectDialog
				open={false}
				onOpenChange={vi.fn()}
				isSubmitting={false}
				error={undefined}
				onSubmit={onSubmit}
			/>,
			{ wrapper: Wrapper },
		);

		rerender(
			<ChatProjectDialog
				project={project}
				open
				onOpenChange={vi.fn()}
				isSubmitting={false}
				error={undefined}
				onSubmit={onSubmit}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).toHaveBeenCalledWith({
			name: project.name,
			description: project.description,
			icon: project.icon,
		});
	});
});
