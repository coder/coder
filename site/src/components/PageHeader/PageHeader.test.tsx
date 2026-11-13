import { render, screen } from "@testing-library/react";
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "./PageHeader";

describe(PageHeader.name, () => {
	it("exposes only the title text as a heading", () => {
		render(
			<PageHeader>
				<PageHeaderTitle tooltip={<button type="button">More info</button>}>
					Templates
				</PageHeaderTitle>
				<PageHeaderSubtitle>Select a template.</PageHeaderSubtitle>
			</PageHeader>,
		);

		expect(screen.getAllByRole("heading").map((h) => h.textContent)).toEqual([
			"Templates",
		]);
	});
});
