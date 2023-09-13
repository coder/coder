import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import * as API from "../../../api/api";
import { renderWithAuth } from "../../../testHelpers/renderHelpers";
import { SchedulePage } from "./SchedulePage";
import i18next from "i18next";

const { t } = i18next;

const renderPage = () => {
  return renderWithAuth(<SchedulePage />);
};

const fillForm = async ({
  hours,
  minutes,
  timezone,
}: {
  hours: number;
  minutes: number;
  timezone: string;
}) => {
  await waitFor(() => screen.findByLabelText("Hours"));
  await waitFor(() => screen.findByLabelText("Minutes"));
  fireEvent.change(screen.getByLabelText("Hours"), {
    target: { value: hours },
  });
  fireEvent.change(screen.getByLabelText("Minutes"), {
    target: { value: minutes },
  });

  await waitFor(() => screen.findByLabelText("Timezone"));
  fireEvent.click(screen.getByLabelText("Timezone"));
  // TODO: fix the options targeting
  const optionsList = screen.getByRole("listbox");
  const option = within(optionsList).getByText(timezone);
  fireEvent.click(option);
};

const readCronExpression = () => {
  return screen.getByLabelText("Cron schedule").getAttribute("value");
};

const readNextOccurrence = () => {
  return screen.getByLabelText("Next occurrence").getAttribute("value");
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
    hours: 0,
    minutes: 0,
    currentTime: new Date("2023-09-06T15:00:00.000+10:00Z"),
    expectedNext: "12:00AM tomorrow (in 9 hours)",
  },
];

describe("SchedulePage", () => {
  describe("cron tests", () => {
    for (let i = 0; i < cronTests.length; i++) {
      const test = cronTests[i];
      describe(`case ${i}`, () => {
        it("has the correct expected time", async () => {
          jest
            .spyOn(API, "getUserQuietHoursSchedule")
            .mockImplementationOnce(() =>
              Promise.resolve(defaultQuietHoursResponse),
            );
          jest
            .spyOn(API, "updateUserQuietHoursSchedule")
            .mockImplementationOnce((userId, data) => {
              return Promise.resolve({
                raw_schedule: data.schedule,
                user_set: true,
                time: `${test.hours.toString().padStart(2, "0")}:${test.minutes
                  .toString()
                  .padStart(2, "0")}`,
                timezone: test.timezone,
                next: "", // This value isn't used in the UI, the UI generates it.
              });
            });
          const { user } = renderPage();

          await fillForm(test);

          const expectedCronSchedule = `CRON_TZ=${test.timezone} ${test.minutes} ${test.hours} * * *`;
          expect(readCronExpression()).toEqual(expectedCronSchedule);
          expect(readNextOccurrence()).toEqual(test.expectedNext);

          await submitForm();
          const successMessage = await screen.findByText(
            "Schedule updated successfully",
          );
          expect(successMessage).toBeDefined();
          expect(API.updateUserQuietHoursSchedule).toBeCalledTimes(1);
          expect(API.updateUserQuietHoursSchedule).toBeCalledWith(user.id, {
            schedule: expectedCronSchedule,
          });
        });
      });
    }
  });

  describe("when it is an unknown error", () => {
    it("shows a generic error message", async () => {
      jest
        .spyOn(API, "getUserQuietHoursSchedule")
        .mockImplementationOnce(() =>
          Promise.resolve(defaultQuietHoursResponse),
        );
      jest.spyOn(API, "updateUserQuietHoursSchedule").mockRejectedValueOnce({
        data: "unknown error",
      });

      renderPage();
      await fillForm(cronTests[0]);
      await submitForm();

      const errorText = t("warningsAndErrors.somethingWentWrong", {
        ns: "common",
      });
      const errorMessage = await screen.findByText(errorText);
      expect(errorMessage).toBeDefined();
    });
  });
});
