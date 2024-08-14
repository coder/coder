import { render, screen } from "@testing-library/react";
import { Abbr, type Pronunciation } from "./Abbr";

type AbbreviationData = {
  abbreviation: string;
  title: string;
  expectedLabel: string;
};

type AssertionInput = AbbreviationData & {
  pronunciation: Pronunciation;
};

function assertAccessibleLabel({
  abbreviation,
  title,
  expectedLabel,
  pronunciation,
}: AssertionInput) {
  const { unmount } = render(
    <Abbr title={title} pronunciation={pronunciation}>
      {abbreviation}
    </Abbr>,
  );

  screen.getByLabelText(expectedLabel, { selector: "abbr" });
  unmount();
}

describe(Abbr.name, () => {
  it("Has an aria-label that equals the title if the abbreviation is shorthand", () => {
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
      assertAccessibleLabel({ ...shorthand, pronunciation: "shorthand" });
    }
  });

  it("Has an aria label with title and 'flattened' pronunciation if abbreviation is acronym", () => {
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
      assertAccessibleLabel({ ...acronym, pronunciation: "acronym" });
    }
  });

  it("Has an aria label with title and initialized pronunciation if abbreviation is initialism", () => {
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
      assertAccessibleLabel({ ...initialism, pronunciation: "initialism" });
    }
  });
});
