import { render, screen } from "@testing-library/react";
import { ExternalAuthItem } from "./ExternalAuthItem";
import { ThemeProvider } from "contexts/ThemeProvider";
import { TemplateVersionExternalAuth } from "api/typesGenerated";
import userEvent from "@testing-library/user-event";

jest.spyOn(window, "open").mockImplementation(() => null);

const MockExternalAuth: TemplateVersionExternalAuth = {
  id: "",
  type: "",
  display_name: "GitHub",
  display_icon: "/icon/github.svg",
  authenticate_url: "",
  authenticated: false,
};

test("changes to idle when polling stops", async () => {
  const user = userEvent.setup();
  const startPollingFn = jest.fn();
  const { rerender } = render(
    <ExternalAuthItem
      isPolling={false}
      provider={MockExternalAuth}
      onStartPolling={startPollingFn}
    />,
    { wrapper: ThemeProvider },
  );

  const connectButton = screen.getByText<HTMLButtonElement>(/connect/i);
  expect(isLoading(connectButton)).toBeFalsy();

  await user.click(connectButton);
  expect(startPollingFn).toHaveBeenCalledTimes(1);
  expect(window.open).toHaveBeenCalledTimes(1);

  rerender(
    <ExternalAuthItem
      isPolling
      provider={MockExternalAuth}
      onStartPolling={startPollingFn}
    />,
  );

  // Check if the button is loading
  screen.getByRole("progressbar");

  rerender(
    <ExternalAuthItem
      isPolling={false}
      provider={MockExternalAuth}
      onStartPolling={startPollingFn}
    />,
  );

  expect(isLoading(connectButton)).toBeFalsy();
});

function isLoading(el: HTMLButtonElement) {
  const progressBar = el.querySelector('[role="progressbar"]');
  return Boolean(progressBar);
}
