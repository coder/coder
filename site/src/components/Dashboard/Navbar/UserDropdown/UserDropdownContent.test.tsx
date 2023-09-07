import { screen } from "@testing-library/react";
import { MockUser } from "testHelpers/entities";
import { render } from "testHelpers/renderHelpers";
import { Language, UserDropdownContent } from "./UserDropdownContent";

describe("UserDropdownContent", () => {
  it("has the correct link for the account item", () => {
    render(
      <UserDropdownContent
        user={MockUser}
        onSignOut={jest.fn()}
        onPopoverClose={jest.fn()}
      />,
    );

    const link = screen.getByText(Language.accountLabel).closest("a");
    if (!link) {
      throw new Error("Anchor tag not found for the account menu item");
    }

    expect(link.getAttribute("href")).toBe("/settings/account");
  });

  it("calls the onSignOut function", () => {
    const onSignOut = jest.fn();
    render(
      <UserDropdownContent
        user={MockUser}
        onSignOut={onSignOut}
        onPopoverClose={jest.fn()}
      />,
    );
    screen.getByText(Language.signOutLabel).click();
    expect(onSignOut).toBeCalledTimes(1);
  });
});
