import { render, screen } from "@testing-library/react";
import { Abbr } from "./Abbr";

type Abbreviation = {
  shortText: string;
  fullText: string;
};

type Initialism = Abbreviation & {
  initializedForm: string;
};

describe(Abbr.name, () => {
  it("Does not change semantics compared <abbr> if text is not initialism", () => {
    const sampleText: Abbreviation[] = [
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

      const element = screen.getByTestId("abbr-root");
      expect(element).toHaveTextContent(shortText);
      unmount();
    }
  });

  it("Augments pronunciation for screen readers if text is an initialism (but does not change visual output)", () => {
    const sampleText: Initialism[] = [
      {
        shortText: "FBI",
        fullText: "Federal Bureau of Investigation",
        initializedForm: "F.B.I.",
      },
      {
        shortText: "YMCA",
        fullText: "Young Men's Christian Association",
        initializedForm: "Y.M.C.A.",
      },
      {
        shortText: "tbh",
        fullText: "To be honest",
        initializedForm: "T.B.H.",
      },
      {
        shortText: "CLI",
        fullText: "Command-Line Interface",
        initializedForm: "C.L.I.",
      },
    ];

    for (const { shortText, fullText, initializedForm } of sampleText) {
      const { unmount } = render(
        <Abbr title={fullText} pronunciation="initialism">
          {shortText}
        </Abbr>,
      );

      const visuallyHidden = screen.getByTestId("abbr-screen-readers");
      expect(visuallyHidden).toHaveTextContent(initializedForm);

      const visualContent = screen.getByTestId("visual-only");
      expect(visualContent).toHaveTextContent(shortText);

      unmount();
    }
  });
});
