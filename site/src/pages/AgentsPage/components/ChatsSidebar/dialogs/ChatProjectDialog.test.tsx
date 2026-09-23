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
