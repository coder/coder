import { render, screen } from "@testing-library/react";
import { AppProviders } from "App";
import { createTestQueryClient } from "testHelpers/renderHelpers";
import { Markdown } from "./Markdown";

const renderWithProviders = (children: React.ReactNode) => {
	return render(
		<AppProviders queryClient={createTestQueryClient()}>
			{children}
		</AppProviders>,
	);
};

describe("Markdown", () => {
	it("renders GFM alerts without links correctly", () => {
		const markdown = `> [!NOTE]
> Useful information that users should know, even when skimming content.`;

		renderWithProviders(<Markdown>{markdown}</Markdown>);

		// Should render as an alert, not a regular blockquote
		const alert = screen.getByRole("complementary");
		expect(alert).toBeInTheDocument();
		expect(alert).toHaveTextContent(
			"Useful information that users should know, even when skimming content.",
		);
	});

	it("renders GFM alerts with links correctly", () => {
		const markdown = `> [!NOTE]
> This template is centrally managed by CI/CD in the [coder/templates](https://github.com/coder/templates) repository.`;

		renderWithProviders(<Markdown>{markdown}</Markdown>);

		// Should render as an alert, not a regular blockquote
		const alert = screen.getByRole("complementary");
		expect(alert).toBeInTheDocument();
		// The alert should contain the content (the alert type might be included)
		expect(alert).toHaveTextContent(
			/This template is centrally managed by CI\/CD in the.*repository/,
		);

		// Should contain the link
		const link = screen.getByRole("link", { name: /coder\/templates/ });
		expect(link).toBeInTheDocument();
		expect(link).toHaveAttribute("href", "https://github.com/coder/templates");
	});

	it("renders multiple GFM alerts with links correctly", () => {
		const markdown = `> [!TIP]
> Check out the [documentation](https://docs.coder.com) for more information.

> [!WARNING]
> This action may affect your [workspace settings](https://coder.com/settings).`;

		renderWithProviders(<Markdown>{markdown}</Markdown>);

		// Should render both alerts
		const alerts = screen.getAllByRole("complementary");
		expect(alerts).toHaveLength(2);

		// Check first alert (TIP)
		expect(alerts[0]).toHaveTextContent(
			/Check out the.*documentation.*for more information/,
		);
		const docLink = screen.getByRole("link", { name: /documentation/ });
		expect(docLink).toHaveAttribute("href", "https://docs.coder.com");

		// Check second alert (WARNING)
		expect(alerts[1]).toHaveTextContent(
			/This action may affect your.*workspace settings/,
		);
		const settingsLink = screen.getByRole("link", {
			name: /workspace settings/,
		});
		expect(settingsLink).toHaveAttribute("href", "https://coder.com/settings");
	});

	it("falls back to regular blockquote for invalid alert types", () => {
		const markdown = `> [!INVALID]
> This should render as a regular blockquote.`;

		renderWithProviders(<Markdown>{markdown}</Markdown>);

		// Should render as a regular blockquote, not an alert
		// Use a more specific selector since blockquote doesn't have an accessible role
		const blockquote = screen.getByText(
			/\[!INVALID\].*This should render as a regular blockquote/,
		);
		expect(blockquote).toBeInTheDocument();
	});

	it("renders regular blockquotes without alert syntax", () => {
		const markdown = `> This is a regular blockquote without alert syntax.`;

		renderWithProviders(<Markdown>{markdown}</Markdown>);

		// Should render as a regular blockquote
		const blockquote = screen.getByText(
			"This is a regular blockquote without alert syntax.",
		);
		expect(blockquote).toBeInTheDocument();
	});
});
