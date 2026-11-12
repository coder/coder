import { render, screen } from "@testing-library/react";
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "./PageHeader";

describe(PageHeader.name, () => {
	it("exposes only the title as a heading", () => {
		render(
			<PageHeader>
				<PageHeaderTitle>Templates</PageHeaderTitle>
				<PageHeaderSubtitle>Select a template.</PageHeaderSubtitle>
			</PageHeader>,
		);

		expect(screen.getAllByRole("heading").map((h) => h.textContent)).toEqual([
			"Templates",
		]);
	});
});
