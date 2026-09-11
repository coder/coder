import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { HarnessConfigMenu } from "./HarnessConfigMenu";
import type { Harness } from "./HarnessPicker";

describe("HarnessConfigMenu", () => {
	it.each([
		{ path: "/work/project", expected: "/work/project" },
		{ path: "", expected: "/home/coder" },
	])("saves working directory '$path'", async ({ path, expected }) => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<AppProviders>
				<HarnessConfigMenu harness="Codex" values={{}} onChange={onChange} />
			</AppProviders>,
		);
		await user.click(
			screen.getByRole("button", { name: "Working directory /home/coder" }),
		);
		const input = screen.getByRole("textbox", { name: "Working directory" });
		await user.clear(input);
		if (path) await user.type(input, path);
		await user.click(screen.getByRole("button", { name: "Save" }));
		expect(onChange).toHaveBeenCalledWith("working_directory", expected);
	});

	it.each(["Cancel", "Escape", "Reset"])(
		"handles directory %s without saving edits implicitly",
		async (action) => {
			const user = userEvent.setup();
			const onChange = vi.fn();
			render(
				<AppProviders>
					<HarnessConfigMenu
						harness="Codex"
						values={{ working_directory: "/work/project" }}
						onChange={onChange}
					/>
				</AppProviders>,
			);
			await user.click(
				screen.getByRole("button", { name: "Working directory /work/project" }),
			);
			const input = screen.getByRole("textbox", { name: "Working directory" });
			await user.clear(input);
			await user.type(input, "/other");
			if (action === "Reset") {
				await user.click(
					screen.getByRole("button", {
						name: "Reset working directory to default",
					}),
				);
				expect(onChange).not.toHaveBeenCalled();
				await user.click(screen.getByRole("button", { name: "Save" }));
				expect(onChange).toHaveBeenCalledWith(
					"working_directory",
					"/home/coder",
				);
			} else {
				if (action === "Cancel")
					await user.click(screen.getByRole("button", { name: "Cancel" }));
				else await user.keyboard("{Escape}");
				expect(onChange).not.toHaveBeenCalled();
				await user.click(
					screen.getByRole("button", {
						name: "Working directory /work/project",
					}),
				);
				await user.keyboard("{Enter}");
				expect(onChange).toHaveBeenCalledWith(
					"working_directory",
					"/work/project",
				);
			}
		},
	);

	it.each<{
		harness: Harness;
		setting: string;
		option: string;
		id: string;
		value: string;
	}>([
		{
			harness: "Codex",
			setting: "Mode Approve for me",
			option: "Full access",
			id: "mode",
			value: "agent-full-access",
		},
		{
			harness: "Codex",
			setting: "Collaboration mode Default",
			option: "Plan",
			id: "collaboration_mode",
			value: "plan",
		},
		{
			harness: "Codex",
			setting: "Model 6 Astra",
			option: "5.6 Sol",
			id: "model",
			value: "gpt-5.6-sol",
		},
		{
			harness: "Codex",
			setting: "Reasoning effort Low",
			option: "High",
			id: "reasoning_effort",
			value: "high",
		},
		{
			harness: "Codex",
			setting: "Fast mode Off",
			option: "On",
			id: "fast-mode",
			value: "on",
		},
		{
			harness: "Claude Code",
			setting: "Permission mode Default",
			option: "Accept edits",
			id: "permission_mode",
			value: "accept-edits",
		},
		{
			harness: "Pi",
			setting: "Tool preset Coding",
			option: "Read only",
			id: "tool_preset",
			value: "read-only",
		},
	])(
		"changes $harness $id",
		async ({ harness, setting, option, id, value }) => {
			const user = userEvent.setup();
			const onChange = vi.fn();
			render(
				<AppProviders>
					<HarnessConfigMenu
						harness={harness}
						values={{}}
						onChange={onChange}
					/>
				</AppProviders>,
			);
			await user.click(screen.getByRole("button", { name: setting }));
			await user.click(
				screen.getByRole("option", {
					name: new RegExp(`^${option.replaceAll(".", "\\.")}( |$)`),
				}),
			);
			expect(onChange).toHaveBeenCalledWith(id, value);
		},
	);
	it("supports keyboard selection from the saved value", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<AppProviders>
				<HarnessConfigMenu
					harness="Codex"
					values={{ reasoning_effort: "high" }}
					onChange={onChange}
				/>
			</AppProviders>,
		);
		await user.click(
			screen.getByRole("button", { name: "Reasoning effort High" }),
		);
		await user.keyboard("{ArrowDown}{Enter}");
		expect(onChange).toHaveBeenCalledWith("reasoning_effort", "xhigh");
	});
});
