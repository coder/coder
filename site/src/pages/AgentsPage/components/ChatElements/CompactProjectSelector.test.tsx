import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ChatProject } from "#/api/typesGenerated";
import { MockChatProject } from "#/testHelpers/entities";
import { CompactProjectSelector } from "./CompactProjectSelector";

const launchProject: ChatProject = {
	...MockChatProject,
	id: "docs-launch",
	name: "Launch",
};
const docsProject: ChatProject = {
	...MockChatProject,
	id: "docs-guide",
	name: "Docs",
};

type SelectorProps = React.ComponentProps<typeof CompactProjectSelector>;

const renderSelector = (props: Partial<SelectorProps> = {}) => {
	const handlers = {
		onChange: vi.fn<SelectorProps["onChange"]>(),
		onCreateProject: vi.fn<SelectorProps["onCreateProject"]>(),
		onRetry: vi.fn<NonNullable<SelectorProps["onRetry"]>>(),
	};
	render(
		<CompactProjectSelector
			value={null}
			options={[launchProject, docsProject]}
			{...handlers}
			{...props}
		/>,
	);
	return handlers;
};

const openSelector = async () => {
	await userEvent.click(screen.getByRole("button", { name: /^Project:/ }));
};

describe("CompactProjectSelector", () => {
	it("selects a project", async () => {
		const { onChange } = renderSelector();

		await openSelector();
		await userEvent.click(await screen.findByRole("option", { name: "Docs" }));

		expect(onChange).toHaveBeenCalledWith(docsProject);
	});

	it("selects no project", async () => {
		const { onChange } = renderSelector({ value: launchProject });

		await openSelector();
		await userEvent.click(
			await screen.findByRole("option", { name: /No project/ }),
		);

		expect(onChange).toHaveBeenCalledWith(null);
	});

	it("opens project creation from the footer", async () => {
		const { onCreateProject } = renderSelector();

		await openSelector();
		await userEvent.click(
			await screen.findByRole("button", { name: "New project" }),
		);

		expect(onCreateProject).toHaveBeenCalledTimes(1);
	});

	it("matches the search against project names, not IDs", async () => {
		const { onChange } = renderSelector();

		await openSelector();
		// Both IDs contain "docs", and Launch comes first; only the Docs name
		// matches.
		await userEvent.type(
			await screen.findByRole("combobox", { name: "Search projects" }),
			"docs{Enter}",
		);

		expect(onChange).toHaveBeenCalledWith(docsProject);
	});

	it("retries loading projects after an error", async () => {
		const { onRetry } = renderSelector({
			options: [],
			error: new Error("Network down"),
		});

		await openSelector();
		await userEvent.click(await screen.findByRole("button", { name: "Retry" }));

		expect(onRetry).toHaveBeenCalledTimes(1);
	});
});
