import { screen } from "@testing-library/react";
import { render } from "../../../testHelpers/renderHelpers";
import { LicenseCard } from "./LicenseCard";
import { MockLicenseResponse } from "testHelpers/entities";

describe("LicenseCard", () => {
  it("renders (smoke test)", async () => {
    // When
    render(
      <LicenseCard
        license={MockLicenseResponse[0]}
        userLimitActual={1}
        userLimitLimit={10}
        onRemove={() => null}
        isRemoving={false}
      />,
    );

    // Then
    await screen.findByText("#1");
    await screen.findByText("1 / 10");
    await screen.findByText("Enterprise");
  });

  it("renders userLimit as unlimited if there is not user limit", async () => {
    // When
    render(
      <LicenseCard
        license={MockLicenseResponse[0]}
        userLimitActual={1}
        userLimitLimit={undefined}
        onRemove={() => null}
        isRemoving={false}
      />,
    );

    // Then
    await screen.findByText("#1");
    await screen.findByText("1 / Unlimited");
    await screen.findByText("Enterprise");
  });

  it("renders license's user_limit when it is available instead of using the default", async () => {
    const licenseUserLimit = 3;
    const license = {
      ...MockLicenseResponse[0],
      claims: {
        ...MockLicenseResponse[0].claims,
        features: {
          ...MockLicenseResponse[0].claims.features,
          user_limit: licenseUserLimit,
        },
      },
    };

    // When
    render(
      <LicenseCard
        license={license}
        userLimitActual={1}
        userLimitLimit={100} // This should not be used
        onRemove={() => null}
        isRemoving={false}
      />,
    );

    // Then
    await screen.findByText("1 / 3");
  });
});
