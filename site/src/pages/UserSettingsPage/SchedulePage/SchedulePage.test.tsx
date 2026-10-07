import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { UpdateUserQuietHoursScheduleRequest } from "#/api/typesGenerated";
import { MockUserOwner } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import SchedulePage from "./SchedulePage";

const fillForm = async ({
	hour,
	minute,
	timezone,
	search = timezone,
	keyboard = false,
}: {
	hour: number;
	minute: number;
	timezone: string;
	search?: string;
	keyboard?: boolean;
}) => {
	const user = userEvent.setup();
	// findByLabelText already retries. Wrapping it in waitFor raced two 1s
	// budgets against each other, so a slow first render timed out.
	await screen.findByLabelText("Start time", undefined, { timeout: 10_000 });
	const HH = hour.toString().padStart(2, "0");
	const mm = minute.toString().padStart(2, "0");
	fireEvent.change(screen.getByLabelText("Start time"), {
		target: { value: `${HH}:${mm}` },
	});

	const timezoneDropdown = screen.getByLabelText("Timezone");
	await user.click(timezoneDropdown);
	await user.type(
		screen.getByRole("combobox", { name: "Search timezones" }),
		search,
	);
	if (keyboard) {
		await user.keyboard("{ArrowDown}{ArrowUp}{Enter}");
	} else {
		const list = screen.getByRole("listbox");
		await user.click(
			within(list).getByRole("option", {
				name: timezone.replaceAll("_", " "),
			}),
		);
	}
};

const submitForm = async () => {
	fireEvent.click(screen.getByText("Update schedule"));
};

const defaultQuietHoursResponse = {
	raw_schedule: "CRON_TZ=America/Chicago 0 0 * * *",
	user_set: false,
	user_can_set: true,
	time: "00:00",
	timezone: "America/Chicago",
	next: "", // not consumed by the frontend
};

const cronTests = [
	{
		timezone: "Australia/Sydney",
		hour: 0,
		minute: 0,
		search: "australia",
	},
	{
		timezone: "America/New_York",
		hour: 7,
		minute: 30,
		search: "new york",
		keyboard: true,
	},
	{
		timezone: "UTC",
		hour: 2,
		minute: 0,
	},
	{
		timezone: "America/Chicago",
		hour: 0,
		minute: 0,
	},
] as const;

describe("SchedulePage", () => {
	beforeEach(() => {
		server.use(
			http.get(`/api/v2/users/${MockUserOwner.id}/quiet-hours`, () => {
				return HttpResponse.json(defaultQuietHoursResponse);
			}),
		);
	});

	describe("cron tests", () => {
		it.each(cronTests)(
			"submits the selected timezone: $timezone",
			async (test) => {
				const onUpdate = vi.fn();
				server.use(
					http.get(`/api/v2/users/${MockUserOwner.id}/quiet-hours`, () => {
						return HttpResponse.json({
							...defaultQuietHoursResponse,
							user_set: true,
						});
					}),
					http.put(
						`/api/v2/users/${MockUserOwner.id}/quiet-hours`,
						async ({ request }) => {
							const data =
								(await request.json()) as UpdateUserQuietHoursScheduleRequest;
							onUpdate(data);
							return HttpResponse.json({
								raw_schedule: data.schedule,
								user_set: true,
								time: `${test.hour.toString().padStart(2, "0")}:${test.minute
									.toString()
									.padStart(2, "0")}`,
								timezone: test.timezone,
								next: "", // This value isn't used in the UI, the UI generates it.
							});
						},
					),
				);

				renderWithAuth(<SchedulePage />);
				await fillForm(test);
				await submitForm();
				await waitFor(() => {
					expect(onUpdate).toHaveBeenCalledExactlyOnceWith({
						schedule: `CRON_TZ=${test.timezone} ${test.minute} ${test.hour} * * *`,
					});
				});
			},
			15_000,
		);
	});

	describe("when it is an unknown error", () => {
		it("shows a generic error message", async () => {
			server.use(
				http.put(`/api/v2/users/${MockUserOwner.id}/quiet-hours`, () => {
					return HttpResponse.json(
						{
							message: "oh no!",
						},
						{ status: 500 },
					);
				}),
			);

			renderWithAuth(<SchedulePage />);
			await fillForm(cronTests[0]);
			await submitForm();

			const errorMessage = await screen.findByText("oh no!");
			expect(errorMessage).toBeDefined();
		}, 15_000);
	});

	describe("when user custom schedule is disabled", () => {
		it("shows a warning and disables the form", async () => {
			server.use(
				http.get(`/api/v2/users/${MockUserOwner.id}/quiet-hours`, () => {
					return HttpResponse.json({
						raw_schedule: "CRON_TZ=America/Chicago 0 0 * * *",
						user_can_set: false,
						user_set: false,
						time: "00:00",
						timezone: "America/Chicago",
						next: "", // not consumed by the frontend
					});
				}),
			);

			renderWithAuth(<SchedulePage />);
			await screen.findByText(
				"Your administrator has disabled the ability to set a custom quiet hours schedule.",
			);

			const timeInput = screen.getByLabelText("Start time");
			expect(timeInput).toBeDisabled();
			const timezoneDropdown = screen.getByLabelText("Timezone");
			expect(timezoneDropdown).toBeDisabled();
			const updateButton = screen.getByText("Update schedule");
			expect(updateButton).toBeDisabled();
		});
	});
});
