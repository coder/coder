import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ThemeOverride } from "contexts/ThemeProvider";
import type { ComponentProps } from "react";
import themes, { DEFAULT_THEME } from "theme";
import type { FileTree } from "utils/filetree";
import { TemplateFileTree } from "./TemplateFileTree";

const fileTree: FileTree = {
	"main.tf": "main",
	"variables.tf": "variables",
	folder: {
		"nested.tf": "nested",
	},
};

const renderTemplateFileTree = (
	props: ComponentProps<typeof TemplateFileTree>,
) => {
	return render(
		<ThemeOverride theme={themes[DEFAULT_THEME]}>
			<TemplateFileTree {...props} />
		</ThemeOverride>,
	);
};

const getTreeItemContent = (label: string) => {
	const content = screen.getByText(label).closest(".MuiTreeItem-content");
	expect(content).not.toBeNull();
	return content as HTMLElement;
};

describe(TemplateFileTree.name, () => {
	it("updates the highlighted item when activePath changes", async () => {
		const onSelect = vi.fn();
		const { rerender } = renderTemplateFileTree({
			fileTree,
			activePath: "main.tf",
			onSelect,
		});

		expect(getTreeItemContent("main.tf")).toHaveClass("Mui-selected");

		rerender(
			<ThemeOverride theme={themes[DEFAULT_THEME]}>
				<TemplateFileTree
					fileTree={fileTree}
					activePath="folder/nested.tf"
					onSelect={onSelect}
				/>
			</ThemeOverride>,
		);

		await waitFor(() => {
			expect(getTreeItemContent("nested.tf")).toHaveClass("Mui-selected");
			expect(getTreeItemContent("nested.tf")).toBeVisible();
		});
		expect(getTreeItemContent("main.tf")).not.toHaveClass("Mui-selected");
	});

	it("keeps manual selection working when activePath is uncontrolled", async () => {
		const onSelect = vi.fn();
		renderTemplateFileTree({ fileTree, onSelect });

		const variablesItem = getTreeItemContent("variables.tf");
		fireEvent.click(variablesItem);

		expect(onSelect).toHaveBeenCalledWith("variables.tf");
		await waitFor(() => {
			expect(variablesItem).toHaveClass("Mui-selected");
		});
	});
});
