import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { SchedulePage } from "./SchedulePage";
import { server } from "testHelpers/server";
import { MockUser } from "testHelpers/entities";
import { rest } from "msw";

const fillForm = async ({
  hour,
  minute,
  timezone,
}: {
  hour: number;
  minute: number;
  timezone: string;
}) => {
  const user = userEvent.setup();
  await waitFor(() => screen.findByLabelText("Start time"));
  const HH = hour.toString().padStart(2, "0");
  const mm = minute.toString().padStart(2, "0");
  fireEvent.change(screen.getByLabelText("Start time"), {
    target: { value: `${HH}:${mm}` },
  });

  const timezoneDropdown = screen.getByLabelText("Timezone");
  await user.click(timezoneDropdown);
  const list = screen.getByRole("listbox");
  const option = within(list).getByText(timezone);
  await user.click(option);
};

const submitForm = async () => {
  fireEvent.click(screen.getByText("Update schedule"));
};

const defaultQuietHoursResponse = {
  raw_schedule: "CRON_TZ=America/Chicago 0 0 * * *",
  user_set: false,
  time: "00:00",
  timezone: "America/Chicago",
  next: "", // not consumed by the frontend
};

const cronTests = [
  {
    timezone: "Australia/Sydney",
    hour: 0,
    minute: 0,
  },
] as const;

describe("SchedulePage", () => {
  beforeEach(() => {
    // appear logged out
    server.use(
      rest.get(`/api/v2/users/${MockUser.id}/quiet-hours`, (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(defaultQuietHoursResponse));
      }),
    );
  });

  describe("cron tests", () => {
    it.each(cronTests)(
      "case %# has the correct expected time",
      async (test) => {
        server.use(
          rest.put(
            `/api/v2/users/${MockUser.id}/quiet-hours`,
            async (req, res, ctx) => {
              const data = await req.json();
              return res(
                ctx.status(200),
                ctx.json({
                  response: {},
                  raw_schedule: data.schedule,
                  user_set: true,
                  time: `${test.hour.toString().padStart(2, "0")}:${test.minute
                    .toString()
                    .padStart(2, "0")}`,
                  timezone: test.timezone,
                  next: "", // This value isn't used in the UI, the UI generates it.
                }),
              );
            },
          ),
        );

        const expectedCronSchedule = `CRON_TZ=${test.timezone} ${test.minute} ${test.hour} * * *`;
        renderWithAuth(<SchedulePage />);
        await fillForm(test);
        const cron = screen.getByLabelText("Cron schedule");
        expect(cron.getAttribute("value")).toEqual(expectedCronSchedule);

        await submitForm();
        const successMessage = await screen.findByText(
          "Schedule updated successfully",
        );
        expect(successMessage).toBeDefined();
      },
    );
  });

  describe("when it is an unknown error", () => {
    it("shows a generic error message", async () => {
      server.use(
        rest.put(
          `/api/v2/users/${MockUser.id}/quiet-hours`,
          (req, res, ctx) => {
            return res(
              ctx.status(500),
              ctx.json({
                message: "oh no!",
              }),
            );
          },
        ),
      );

      renderWithAuth(<SchedulePage />);
      await fillForm(cronTests[0]);
      await submitForm();

      const errorMessage = await screen.findByText("oh no!");
      expect(errorMessage).toBeDefined();
    });
  });
});
