import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, onTestFinished, vi } from "vitest";
import { createSkillMenuItem, SkillsTriggerMenu } from "./SkillsTriggerMenu";

describe("createSkillMenuItem", () => {
	it("qualifies plugin skills as /plugin/<plugin>/<name>", () => {
		const item = createSkillMenuItem("plugin", {
			name: "deploy",
			description: "Deploy via acme",
			pluginName: "acme",
		});
		expect(item.source).toBe("plugin");
		expect(item.pluginName).toBe("acme");
		expect(item.triggerText).toBe("/plugin/acme/deploy");
		expect(item.altTriggerText).toBe("/plugin/acme/deploy");
	});

	it("qualifies workspace skills as /workspace/<name>", () => {
		const item = createSkillMenuItem("workspace", {
			name: "deploy",
			description: "",
		});
		expect(item.triggerText).toBe("/workspace/deploy");
		expect(item.altTriggerText).toBe("/workspace/deploy");
	});

	it("keeps personal skills bare unless asked to qualify", () => {
		const skill = { name: "deploy", description: "" };
		expect(createSkillMenuItem("personal", skill).triggerText).toBe("/deploy");
		expect(createSkillMenuItem("personal", skill, true).triggerText).toBe(
			"/personal/deploy",
		);
	});
});

describe("SkillsTriggerMenu", () => {
	it("selects a plugin skill with its plugin-qualified trigger", async () => {
		const pluginItem = createSkillMenuItem("plugin", {
			name: "deploy",
			description: "Deploy via acme",
			pluginName: "acme",
		});
		const workspaceItem = createSkillMenuItem("workspace", {
			name: "deploy",
			description: "Deploy from the workspace",
		});
		const onSelect = vi.fn();
		const anchor = document.createElement("div");
		document.body.appendChild(anchor);
		onTestFinished(() => anchor.remove());

		render(
			<SkillsTriggerMenu
				open
				anchor={anchor}
				query=""
				personalSkills={[]}
				workspaceSkills={[workspaceItem, pluginItem]}
				workspaceSkillsEnabled
				selectedIndex={0}
				onSelectedIndexChange={vi.fn()}
				onSelect={onSelect}
				onClose={vi.fn()}
			/>,
		);

		await userEvent.click(
			screen.getByRole("option", { name: /\/plugin\/acme\/deploy/ }),
		);

		expect(onSelect).toHaveBeenCalledTimes(1);
		expect(onSelect).toHaveBeenCalledWith(pluginItem);
		expect(onSelect.mock.calls[0][0].triggerText).toBe("/plugin/acme/deploy");
		expect(onSelect.mock.calls[0][0].pluginName).toBe("acme");
	});
});
