import { render, screen } from "@testing-library/react";
import { Abbr } from "./Abbr";

type AbbrEntry = {
  shortText: string;
  fullText: string;
  augmented?: string;
};

describe(Abbr.name, () => {
  it("Does not change visual output compared <abbr> if text is not initialism", () => {
    const sampleText: AbbrEntry[] = [
      {
        shortText: "NASA",
        fullText: "National Aeronautics and Space Administration",
      },
      {
        shortText: "POTUS",
        fullText: "President of the United States",
      },
      {
        shortText: "AWOL",
        fullText: "Absent without Official Leave",
      },
      {
        shortText: "Laser",
        fullText: "Light Amplification by Stimulated Emission of Radiation",
      },
      {
        shortText: "YOLO",
        fullText: "You Only Live Once",
      },
    ];

    for (const { shortText, fullText } of sampleText) {
      const { unmount } = render(<Abbr title={fullText}>{shortText}</Abbr>);

      const element = screen.getByTestId("abbr");
      const matcher = new RegExp(`^${shortText}$`);
      expect(element).toHaveTextContent(matcher);

      unmount();
    }
  });

  it("Augments pronunciation for screen readers if text is an initialism (but does not change visual output)", () => {
    const sampleText: AbbrEntry[] = [
      {
        shortText: "FBI",
        fullText: "Federal Bureau of Investigations",
        augmented: "F.B.I.",
      },
      {
        shortText: "YMCA",
        fullText: "Young Men's Christian Association",
        augmented: "Y.M.C.A.",
      },
      {
        shortText: "tbh",
        fullText: "To be honest",
        augmented: "T.B.H.",
      },
    ];

    for (const { shortText, fullText, augmented } of sampleText) {
      const { unmount } = render(
        <Abbr title={fullText} initialism>
          {shortText}
        </Abbr>,
      );

      const visuallyHidden = screen.getByTestId("visually-hidden");
      expect(visuallyHidden).toHaveTextContent(augmented ?? "");

      const visualContent = screen.getByTestId("visual-only");
      expect(visualContent).toHaveTextContent(shortText);

      unmount();
    }
  });
});
