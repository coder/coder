import { ThemeProvider as EmotionThemeProvider } from "@emotion/react";
import { createTheme, ThemeProvider as MuiThemeProvider } from "@mui/material";
import { render, screen, within } from "@testing-library/react";
import { Markdown } from "./Markdown";

const testTheme = createTheme();

const renderMarkdown = (markdown: string) => {
	return render(
		<MuiThemeProvider theme={testTheme}>
			<EmotionThemeProvider theme={testTheme}>
				<Markdown>{markdown}</Markdown>
			</EmotionThemeProvider>
		</MuiThemeProvider>,
	);
};

describe("Markdown", () => {
	it("renders list blocks inside GFM alerts", () => {
		renderMarkdown(`
> [!NOTE]
> Check these items:
>
> - First item
> - Second item
`);

		const alert = screen.getByText("Note").closest("aside");
		expect(alert).not.toBeNull();
		expect(within(alert!).getByText("Check these items:")).toBeInTheDocument();
		expect(within(alert!).getByRole("list")).toBeInTheDocument();
		expect(
			within(alert!).getByRole("listitem", { name: "First item" }),
		).toBeInTheDocument();
		expect(
			within(alert!).getByRole("listitem", { name: "Second item" }),
		).toBeInTheDocument();
	});

	it("renders fenced bash code blocks inside GFM alerts", () => {
		renderMarkdown(`
> [!WARNING]
> Run this command:
>
> \`\`\`bash
> coder version
> \`\`\`
`);

		const alert = screen.getByText("Warning").closest("aside");
		expect(alert).not.toBeNull();
		expect(within(alert!).getByText("Run this command:")).toBeInTheDocument();
		expect(alert).toHaveTextContent("coder version");
		expect(alert!.querySelector(".language-bash")).toBeInTheDocument();
	});
});
