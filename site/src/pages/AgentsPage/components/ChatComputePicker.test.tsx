import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { MockWorkspace } from "#/testHelpers/entities";
import { ChatComputePicker } from "./ChatComputePicker";

describe("ChatComputePicker", () => {
	it("selects an existing workspace", async () => {
		const user = userEvent.setup();
		const onWorkspaceChange = vi.fn();
		const onTemplateChange = vi.fn();
		render(
			<AppProviders>
				<ChatComputePicker
					workspaceOptions={[MockWorkspace]}
					onWorkspaceChange={onWorkspaceChange}
					onTemplateChange={onTemplateChange}
				/>
			</AppProviders>,
		);
		await user.click(screen.getByRole("button", { name: "Compute" }));
		await user.click(
			screen.getByRole("option", { name: new RegExp(MockWorkspace.name) }),
		);
		expect(onWorkspaceChange).toHaveBeenCalledWith(MockWorkspace.id);
		expect(onTemplateChange).not.toHaveBeenCalled();
	});
	it("selects a template when no workspace is available", async () => {
		const user = userEvent.setup();
		const onWorkspaceChange = vi.fn();
		const onTemplateChange = vi.fn();
		render(
			<AppProviders>
				<ChatComputePicker
					workspaceOptions={[]}
					onWorkspaceChange={onWorkspaceChange}
					onTemplateChange={onTemplateChange}
				/>
			</AppProviders>,
		);
		await user.click(screen.getByRole("button", { name: "Compute" }));
		await user.type(
			screen.getByRole("combobox", { name: "Search compute" }),
			"Python",
		);
		await user.keyboard("{Enter}");
		expect(onTemplateChange).toHaveBeenCalledWith("python");
		expect(onWorkspaceChange).not.toHaveBeenCalled();
	});
	it("does not select a workspace from another organization", async () => {
		const user = userEvent.setup();
		const onWorkspaceChange = vi.fn();
		render(
			<AppProviders>
				<ChatComputePicker
					workspaceOptions={[MockWorkspace]}
					chatOrganizationId="other-organization"
					onWorkspaceChange={onWorkspaceChange}
					onTemplateChange={vi.fn()}
				/>
			</AppProviders>,
		);
		await user.click(screen.getByRole("button", { name: "Compute" }));
		await user.click(
			screen.getByRole("option", { name: new RegExp(MockWorkspace.name) }),
		);
		expect(onWorkspaceChange).not.toHaveBeenCalled();
	});
});
