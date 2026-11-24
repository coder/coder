import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FileTree } from "#/utils/filetree";
import { TemplateFileTree } from "./TemplateFileTree";

const fileTree: FileTree = {
	"main.tf": "resource {}",
	modules: {
		"vars.tf": "variable {}",
		nested: { "deep.tf": "output {}" },
	},
	"README.md": "# Readme",
};

describe("TemplateFileTree", () => {
	it("moves focus with the arrow keys and selects with Enter", async () => {
		const user = userEvent.setup();
		const onSelect = vi.fn();
		render(
			<TemplateFileTree
				fileTree={fileTree}
				activePath="main.tf"
				onSelect={onSelect}
			/>,
		);

		await user.tab();
		await user.keyboard("{Home}{ArrowRight}{ArrowDown}{ArrowDown}{Enter}");
		expect(onSelect).toHaveBeenLastCalledWith("modules/vars.tf");

		// Left goes to the parent folder, then collapses it.
		await user.keyboard("{ArrowLeft}{ArrowLeft}{ArrowDown}{Enter}");
		expect(onSelect).toHaveBeenLastCalledWith("main.tf");

		await user.keyboard("{Home}{ArrowRight}{ArrowDown}{Enter}");
		expect(onSelect).toHaveBeenLastCalledWith("modules/nested");
	});

	it("keeps a single tab stop on the selected file", async () => {
		const user = userEvent.setup();
		render(
			<>
				<TemplateFileTree
					fileTree={fileTree}
					activePath="README.md"
					onSelect={vi.fn()}
					onRename={vi.fn()}
					onDelete={vi.fn()}
				/>
				<button type="button">After</button>
			</>,
		);

		await user.tab();
		expect(screen.getByRole("treeitem", { name: "README.md" })).toHaveFocus();
		await user.tab();
		expect(screen.getByRole("button", { name: "After" })).toHaveFocus();
	});

	it("runs rename and delete from the keyboard", async () => {
		const user = userEvent.setup();
		const onRename = vi.fn();
		const onDelete = vi.fn();
		render(
			<TemplateFileTree
				fileTree={fileTree}
				activePath="main.tf"
				onSelect={vi.fn()}
				onRename={onRename}
				onDelete={onDelete}
			/>,
		);

		await user.tab();
		await user.keyboard("{F2}");
		expect(onRename).toHaveBeenCalledWith("main.tf");
		await user.keyboard("{Delete}");
		expect(onDelete).toHaveBeenCalledWith("main.tf");

		await user.keyboard("{Shift>}{F10}{/Shift}");
		await user.click(await screen.findByRole("menuitem", { name: "Rename" }));
		expect(onRename).toHaveBeenCalledTimes(2);
		expect(screen.getByRole("treeitem", { name: "main.tf" })).toHaveFocus();
	});
});
