import { render, screen } from "@testing-library/react";
import { Abbr } from "./Abbr";

type AbbreviationData = {
	abbreviation: string;
	title: string;
	expectedLabel: string;
};

describe(Abbr.name, () => {
	it("Omits abbreviation from screen-reader output if it is shorthand", () => {
		const sampleShorthands: AbbreviationData[] = [
			{
				abbreviation: "ms",
				title: "milliseconds",
				expectedLabel: "milliseconds",
			},
			{
				abbreviation: "g",
				title: "grams",
				expectedLabel: "grams",
			},
		];

		for (const shorthand of sampleShorthands) {
			const { unmount } = render(
				<Abbr title={shorthand.title} pronunciation="shorthand">
					{shorthand.abbreviation}
				</Abbr>,
			);

			// The <abbr> element doesn't have any ARIA role semantics baked in,
			// so we have to get a little bit more creative with making sure the
			// expected content is on screen in an accessible way
			const element = screen.getByTitle(shorthand.title);
			expect(element).toHaveTextContent(shorthand.expectedLabel);
			unmount();
		}
	});

	it("Adds title and 'flattened' pronunciation if abbreviation is acronym", () => {
		const sampleAcronyms: AbbreviationData[] = [
			{
				abbreviation: "NASA",
				title: "National Aeronautics and Space Administration",
				expectedLabel: "Nasa (National Aeronautics and Space Administration)",
			},
			{
				abbreviation: "AWOL",
				title: "Absent without Official Leave",
				expectedLabel: "Awol (Absent without Official Leave)",
			},
			{
				abbreviation: "YOLO",
				title: "You Only Live Once",
				expectedLabel: "Yolo (You Only Live Once)",
			},
		];

		for (const acronym of sampleAcronyms) {
			const { unmount } = render(
				<Abbr title={acronym.title} pronunciation="acronym">
					{acronym.abbreviation}
				</Abbr>,
			);

			const element = screen.getByTitle(acronym.title);
			expect(element).toHaveTextContent(acronym.expectedLabel);
			unmount();
		}
	});

	it("Adds title and initialized pronunciation if abbreviation is initialism", () => {
		const sampleInitialisms: AbbreviationData[] = [
			{
				abbreviation: "FBI",
				title: "Federal Bureau of Investigation",
				expectedLabel: "F.B.I. (Federal Bureau of Investigation)",
			},
			{
				abbreviation: "YMCA",
				title: "Young Men's Christian Association",
				expectedLabel: "Y.M.C.A. (Young Men's Christian Association)",
			},
			{
				abbreviation: "CLI",
				title: "Command-Line Interface",
				expectedLabel: "C.L.I. (Command-Line Interface)",
			},
		];

		for (const initialism of sampleInitialisms) {
			const { unmount } = render(
				<Abbr title={initialism.title} pronunciation="initialism">
					{initialism.abbreviation}
				</Abbr>,
			);

			const element = screen.getByTitle(initialism.title);
			expect(element).toHaveTextContent(initialism.expectedLabel);
			unmount();
		}
	});
});
