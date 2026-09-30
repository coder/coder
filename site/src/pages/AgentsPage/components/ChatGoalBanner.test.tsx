import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatGoal } from "#/testHelpers/chatEntities";
import { ChatGoalBanner } from "./ChatGoalBanner";

const goal = (
	overrides: Partial<TypesGen.ChatGoal> = {},
): TypesGen.ChatGoal => ({
	...MockChatGoal,
	...overrides,
});

describe("ChatGoalBanner", () => {
	it("sends each action offered for an active goal", async () => {
		const user = userEvent.setup();
		const onAction = vi.fn();
		render(<ChatGoalBanner goal={goal()} canMutateGoal onAction={onAction} />);

		await user.click(screen.getByRole("button", { name: /Pause/i }));
		await user.click(screen.getByRole("button", { name: /Complete/i }));
		await user.click(screen.getByRole("button", { name: /Clear/i }));

		expect(onAction).toHaveBeenNthCalledWith(1, "pause");
		expect(onAction).toHaveBeenNthCalledWith(2, "complete");
		expect(onAction).toHaveBeenNthCalledWith(3, "clear");
	});

	it("sends resume and clear for a paused goal", async () => {
		const user = userEvent.setup();
		const onAction = vi.fn();
		render(
			<ChatGoalBanner
				goal={goal({ status: "paused" })}
				canMutateGoal
				onAction={onAction}
			/>,
		);

		await user.click(screen.getByRole("button", { name: /Resume/i }));
		await user.click(screen.getByRole("button", { name: /Clear/i }));

		expect(onAction).toHaveBeenNthCalledWith(1, "resume");
		expect(onAction).toHaveBeenNthCalledWith(2, "clear");
	});

	it("sends resume and clear for a blocked goal", async () => {
		const user = userEvent.setup();
		const onAction = vi.fn();
		render(
			<ChatGoalBanner
				goal={goal({ status: "blocked", blocked_reason: "Need a decision." })}
				canMutateGoal
				onAction={onAction}
			/>,
		);

		await user.click(screen.getByRole("button", { name: /Resume/i }));
		await user.click(screen.getByRole("button", { name: /Clear/i }));

		expect(onAction).toHaveBeenNthCalledWith(1, "resume");
		expect(onAction).toHaveBeenNthCalledWith(2, "clear");
	});

	it("sends clear for a complete goal", async () => {
		const user = userEvent.setup();
		const onAction = vi.fn();
		render(
			<ChatGoalBanner
				goal={goal({ status: "complete", completion_summary: "Done." })}
				canMutateGoal
				onAction={onAction}
			/>,
		);

		await user.click(screen.getByRole("button", { name: /Clear/i }));

		expect(onAction).toHaveBeenCalledWith("clear");
	});

	it("keeps an unavailable action focusable but inert", async () => {
		const user = userEvent.setup();
		const onAction = vi.fn();
		const reason =
			"The chat is busy. Resume becomes available when it is idle.";
		render(
			<ChatGoalBanner
				goal={goal({ status: "paused" })}
				canMutateGoal
				actionUnavailableReasons={{ resume: reason }}
				onAction={onAction}
			/>,
		);

		const resume = screen.getByRole("button", { name: /Resume/i });
		expect(resume).not.toBeDisabled();
		expect(resume).toHaveAttribute("aria-disabled", "true");
		expect(resume).toHaveAccessibleDescription(reason);
		resume.focus();
		expect(resume).toHaveFocus();

		await user.click(resume);
		expect(onAction).not.toHaveBeenCalled();

		await user.click(screen.getByRole("button", { name: /Clear/i }));
		expect(onAction).toHaveBeenCalledWith("clear");
	});
});
