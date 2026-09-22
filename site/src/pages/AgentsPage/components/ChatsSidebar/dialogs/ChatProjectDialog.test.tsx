import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
import { describe, expect, it, vi } from "vitest";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { MockChatProject } from "#/testHelpers/entities";
import themes, { DEFAULT_THEME } from "#/theme";
import { ChatProjectDialog } from "./ChatProjectDialog";

// The icon field renders external images, which read the active theme.
const Wrapper: FC<PropsWithChildren> = ({ children }) => (
	<ThemeOverride theme={themes[DEFAULT_THEME]}>{children}</ThemeOverride>
);

describe("ChatProjectDialog", () => {
	it("submits a create request", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(async () => {});
		render(
			<ChatProjectDialog
				organizationId="organization-1"
				open
				onOpenChange={vi.fn()}
				onSubmit={onSubmit}
			/>,
			{ wrapper: Wrapper },
		);

		await user.type(screen.getByLabelText("Name"), "Launch");
		await user.type(screen.getByLabelText("Description"), "Release work");
		await user.type(screen.getByLabelText("Icon"), "/emojis/1f680.png");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).toHaveBeenCalledWith({
			organization_id: "organization-1",
			name: "Launch",
			description: "Release work",
			icon: "/emojis/1f680.png",
		});
	});

	it("submits the selected project's name, description, and icon", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(async () => {});
		const project = { ...MockChatProject, icon: "/emojis/1f4c1.png" };
		const { rerender } = render(
			<ChatProjectDialog
				organizationId={project.organization_id}
				open={false}
				onOpenChange={vi.fn()}
				onSubmit={onSubmit}
			/>,
			{ wrapper: Wrapper },
		);

		rerender(
			<ChatProjectDialog
				organizationId={project.organization_id}
				project={project}
				open
				onOpenChange={vi.fn()}
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
