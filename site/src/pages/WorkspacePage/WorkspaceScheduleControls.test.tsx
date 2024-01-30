import { render, screen } from "@testing-library/react";
import { ThemeProvider } from "contexts/ThemeProvider";
import { QueryClient, QueryClientProvider, useQuery } from "react-query";
import { MockWorkspace } from "testHelpers/entities";
import { WorkspaceScheduleControls } from "./WorkspaceScheduleControls";
import { workspaceByOwnerAndName } from "api/queries/workspaces";
import { RouterProvider, createMemoryRouter } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { server } from "testHelpers/server";
import { rest } from "msw";
import dayjs from "dayjs";
import * as API from "api/api";
import { GlobalSnackbar } from "components/GlobalSnackbar/GlobalSnackbar";

const Wrapper = () => {
  const { data: workspace } = useQuery(
    workspaceByOwnerAndName(MockWorkspace.owner_name, MockWorkspace.name),
  );

  if (!workspace) {
    return null;
  }

  return <WorkspaceScheduleControls workspace={workspace} canUpdateSchedule />;
};

test("add 3 hours to deadline", async () => {
  const user = userEvent.setup();
  const baseDeadline = dayjs().add(3, "hour");
  const updateDeadlineSpy = jest
    .spyOn(API, "putWorkspaceExtension")
    .mockResolvedValue();
  server.use(
    rest.get(
      "/api/v2/users/:username/workspace/:workspaceName",
      (req, res, ctx) => {
        return res(
          ctx.status(200),
          ctx.json({
            ...MockWorkspace,
            latest_build: {
              ...MockWorkspace.latest_build,
              deadline: baseDeadline.toISOString(),
            },
          }),
        );
      },
    ),
  );
  render(
    <ThemeProvider>
      <QueryClientProvider client={new QueryClient()}>
        <RouterProvider
          router={createMemoryRouter([{ path: "/", element: <Wrapper /> }])}
        />
      </QueryClientProvider>
      <GlobalSnackbar />
    </ThemeProvider>,
  );
  await screen.findByTestId("schedule-controls");

  // Check if base deadline is displayed correctly
  expect(screen.getByText("Stop in 3 hours")).toBeInTheDocument();

  const addButton = screen.getByLabelText("Add hour");
  await user.click(addButton);
  await user.click(addButton);
  await user.click(addButton);
  await screen.findByText(
    "Workspace shutdown time has been successfully updated.",
  );
  expect(updateDeadlineSpy).toHaveBeenCalledTimes(1);
  expect(screen.getByText("Stop in 6 hours")).toBeInTheDocument();

  // Mocks are used here because the 'usedDeadline' is a dayjs object, which
  // can't be directly compared.
  const usedWorkspaceId = updateDeadlineSpy.mock.calls[0][0];
  const usedDeadline = updateDeadlineSpy.mock.calls[0][1];
  expect(usedWorkspaceId).toEqual(MockWorkspace.id);
  expect(usedDeadline.toISOString()).toEqual(
    baseDeadline.add(3, "hour").toISOString(),
  );
});

test("remove 3 hours to deadline", async () => {
  const user = userEvent.setup();
  const baseDeadline = dayjs().add(3, "hour");
  const updateDeadlineSpy = jest
    .spyOn(API, "putWorkspaceExtension")
    .mockResolvedValue();
  server.use(
    rest.get(
      "/api/v2/users/:username/workspace/:workspaceName",
      (req, res, ctx) => {
        return res(
          ctx.status(200),
          ctx.json({
            ...MockWorkspace,
            latest_build: {
              ...MockWorkspace.latest_build,
              deadline: baseDeadline.toISOString(),
            },
          }),
        );
      },
    ),
  );
  render(
    <ThemeProvider>
      <QueryClientProvider client={new QueryClient()}>
        <RouterProvider
          router={createMemoryRouter([{ path: "/", element: <Wrapper /> }])}
        />
      </QueryClientProvider>
      <GlobalSnackbar />
    </ThemeProvider>,
  );
  await screen.findByTestId("schedule-controls");

  // Check if base deadline is displayed correctly
  expect(screen.getByText("Stop in 3 hours")).toBeInTheDocument();

  const subButton = screen.getByLabelText("Subtract hour");
  await user.click(subButton);
  await user.click(subButton);
  await screen.findByText(
    "Workspace shutdown time has been successfully updated.",
  );
  expect(updateDeadlineSpy).toHaveBeenCalledTimes(1);
  expect(screen.getByText("Stop in an hour")).toBeInTheDocument();

  // Mocks are used here because the 'usedDeadline' is a dayjs object, which
  // can't be directly compared.
  const usedWorkspaceId = updateDeadlineSpy.mock.calls[0][0];
  const usedDeadline = updateDeadlineSpy.mock.calls[0][1];
  expect(usedWorkspaceId).toEqual(MockWorkspace.id);
  expect(usedDeadline.toISOString()).toEqual(
    baseDeadline.subtract(2, "hour").toISOString(),
  );
});

test("rollback to previous deadline on error", async () => {
  const user = userEvent.setup();
  const baseDeadline = dayjs().add(3, "hour");
  const updateDeadlineSpy = jest
    .spyOn(API, "putWorkspaceExtension")
    .mockRejectedValue({});
  server.use(
    rest.get(
      "/api/v2/users/:username/workspace/:workspaceName",
      (req, res, ctx) => {
        return res(
          ctx.status(200),
          ctx.json({
            ...MockWorkspace,
            latest_build: {
              ...MockWorkspace.latest_build,
              deadline: baseDeadline.toISOString(),
            },
          }),
        );
      },
    ),
  );
  render(
    <ThemeProvider>
      <QueryClientProvider client={new QueryClient()}>
        <RouterProvider
          router={createMemoryRouter([{ path: "/", element: <Wrapper /> }])}
        />
      </QueryClientProvider>
      <GlobalSnackbar />
    </ThemeProvider>,
  );
  await screen.findByTestId("schedule-controls");

  // Check if base deadline is displayed correctly
  expect(screen.getByText("Stop in 3 hours")).toBeInTheDocument();

  const addButton = screen.getByLabelText("Add hour");
  await user.click(addButton);
  await user.click(addButton);
  await user.click(addButton);
  await screen.findByText(
    "We couldn't update your workspace shutdown time. Please try again.",
  );
  expect(updateDeadlineSpy).toHaveBeenCalledTimes(1);
  expect(screen.getByText("Stop in 3 hours")).toBeInTheDocument();
});
