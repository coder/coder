import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { preferenceSettingsKey } from "#/api/queries/users";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import { AgentChatInput } from "./AgentChatInput";

type AgentChatInputProps = ComponentProps<typeof AgentChatInput>;

const defaultProps: AgentChatInputProps = {
	onSend: vi.fn(),
	onContentChange: vi.fn(),
	onModelChange: vi.fn(),
	initialValue: "",
	isDisabled: false,
	isLoading: false,
	selectedModel: "model-config-1",
	modelOptions: [
		{
			id: "model-config-1",
			provider: "openai",
			model: "gpt-4o",
			displayName: "GPT-4o",
		},
	],
	modelSelectorPlaceholder: "Select model",
	hasModelOptions: true,
	canConfigureAgentSetup: false,
	showPursueGoal: true,
	canPursueGoal: true,
	onPlanModeToggle: vi.fn(),
};

const renderInput = (props: Partial<AgentChatInputProps>) => {
	const queryClient = createTestQueryClient();
	queryClient.setQueryData(preferenceSettingsKey, MockUserPreferenceSettings);
	const renderTree = (overrides: Partial<AgentChatInputProps>) => (
		<ThemeOverride theme={themes[DEFAULT_THEME]}>
			<TooltipProvider>
				<QueryClientProvider client={queryClient}>
					<AgentChatInput {...defaultProps} {...props} {...overrides} />
				</QueryClientProvider>
			</TooltipProvider>
		</ThemeOverride>
	);
	const view = render(renderTree({}));
	return {
		rerenderInput: (nextProps: Partial<AgentChatInputProps>) =>
			view.rerender(renderTree(nextProps)),
	};
};

const openMoreOptions = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("button", { name: "More options" }));
};

const pursueGoalItem = () =>
	screen.getByRole("menuitemcheckbox", { name: "Pursue goal" });

describe("AgentChatInput goal mode", () => {
	beforeEach(() => {
		vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
			MockUserPreferenceSettings,
		);
	});

	it("sends the trimmed objective as a goal mutation and resets goal mode", async () => {
		const user = userEvent.setup();
		const onSend = vi.fn().mockResolvedValue(undefined);
		renderInput({ onSend, initialValue: "  stabilize the release  " });

		await openMoreOptions(user);
		await user.click(pursueGoalItem());

		await openMoreOptions(user);
		expect(pursueGoalItem()).toHaveAttribute("aria-checked", "true");
		expect(
			screen.getByRole("menuitemcheckbox", { name: "Plan first" }),
		).toHaveAttribute("aria-disabled", "true");
		await user.keyboard("{Escape}");

		const send = screen.getByRole("button", { name: "Send" });
		await waitFor(() => expect(send).toBeEnabled());
		await user.click(send);

		await waitFor(() => {
			expect(onSend).toHaveBeenCalledWith("stabilize the release", {
				goalMutation: { action: "set", objective: "stabilize the release" },
			});
		});

		await openMoreOptions(user);
		expect(pursueGoalItem()).toHaveAttribute("aria-checked", "false");
	});

	it("ignores clicks on the pursue-goal item while goal setting is unavailable", async () => {
		const user = userEvent.setup();
		renderInput({ canPursueGoal: false });

		await openMoreOptions(user);
		const pursueGoal = pursueGoalItem();
		expect(pursueGoal).toHaveAttribute("aria-disabled", "true");
		pursueGoal.focus();
		expect(pursueGoal).toHaveFocus();
		await user.click(pursueGoal);
		expect(pursueGoal).toHaveAttribute("aria-checked", "false");
	});

	it("clears goal mode when the requested plan-mode disable fails", async () => {
		const user = userEvent.setup();
		const onPlanModeToggle = vi
			.fn()
			.mockRejectedValue(new Error("patch failed"));
		renderInput({ planModeEnabled: true, onPlanModeToggle });

		await openMoreOptions(user);
		await user.click(pursueGoalItem());
		expect(onPlanModeToggle).toHaveBeenCalledWith(false);

		// The failed disable leaves plan mode on, so goal mode must clear
		// instead of presenting both modes together.
		await openMoreOptions(user);
		await waitFor(() =>
			expect(pursueGoalItem()).toHaveAttribute("aria-checked", "false"),
		);
	});

	it("drops goal mode when availability is lost and does not reactivate on return", async () => {
		const user = userEvent.setup();
		const onSend = vi.fn().mockResolvedValue(undefined);
		const { rerenderInput } = renderInput({
			onSend,
			initialValue: "Run focused tests",
		});

		await openMoreOptions(user);
		await user.click(pursueGoalItem());

		rerenderInput({ canPursueGoal: false });
		rerenderInput({ canPursueGoal: true });

		const send = screen.getByRole("button", { name: "Send" });
		await waitFor(() => expect(send).toBeEnabled());
		await user.click(send);

		await waitFor(() => {
			expect(onSend).toHaveBeenCalledWith("Run focused tests", undefined);
		});
	});
});
