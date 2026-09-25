import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
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
const Wrapper: FC<PropsWithChildren> = ({ children }) => (
	<ThemeOverride theme={themes[DEFAULT_THEME]}>{children}</ThemeOverride>
);

describe("ChatProjectDialog", () => {
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
		expect(screen.queryByTestId("compact-org-selector")).toBeNull();
	});

	it("submits the only organization without a picker", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		render(
			<ChatProjectDialog
				open
				organizations={[MockOrganization2]}
				onOpenChange={vi.fn()}
				isSubmitting={false}
				error={undefined}
				onSubmit={onSubmit}
			/>,
			{ wrapper: Wrapper },
		);

		expect(screen.queryByTestId("compact-org-selector")).toBeNull();
		await user.type(screen.getByRole("textbox", { name: /Name/ }), "Notes");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).toHaveBeenCalledWith({
			name: "Notes",
			description: "",
			icon: "",
			organization_id: MockOrganization2.id,
		});
	});

	it("submits the organization chosen in the picker", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		render(
			<ChatProjectDialog
				open
				organizations={[MockDefaultOrganization, MockOrganization2]}
				onOpenChange={vi.fn()}
				isSubmitting={false}
				error={undefined}
				onSubmit={onSubmit}
			/>,
			{ wrapper: Wrapper },
		);

		await user.click(
			screen.getByRole("button", {
				name: `Organization: ${MockDefaultOrganization.display_name}`,
			}),
		);
		await user.click(
			screen.getByRole("option", { name: MockOrganization2.display_name }),
		);
		await user.type(screen.getByRole("textbox", { name: /Name/ }), "Notes");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).toHaveBeenCalledWith({
			name: "Notes",
			description: "",
			icon: "",
			organization_id: MockOrganization2.id,
		});
	});
});
