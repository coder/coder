import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import { workspaceByOwnerAndName } from "api/queries/workspaces";
import { add, sub } from "date-fns";
import { http, HttpResponse } from "msw";
import type { FC } from "react";
import { useQuery } from "react-query";
import { MockTemplate, MockWorkspace } from "testHelpers/entities";
import { render } from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { WorkspaceScheduleControls } from "./WorkspaceScheduleControls";

const Wrapper: FC = () => {
	const { data: workspace } = useQuery(
		workspaceByOwnerAndName(MockWorkspace.owner_name, MockWorkspace.name),
	);

	if (!workspace) {
		return null;
	}

	return (
		<WorkspaceScheduleControls
			workspace={workspace}
			template={MockTemplate}
			canUpdateSchedule
		/>
	);
};

const BASE_DEADLINE = add(new Date(), { hours: 3 });

const renderScheduleControls = async () => {
	server.use(
		http.get("/api/v2/users/:username/workspace/:workspaceName", () => {
			return HttpResponse.json({
				...MockWorkspace,
				latest_build: {
					...MockWorkspace.latest_build,
					deadline: BASE_DEADLINE.toISOString(),
				},
			});
		}),
	);
	render(<Wrapper />);
	await screen.findByTestId("schedule-controls");
	expect(screen.getByText(/Stop in (about )?3 hours/)).toBeInTheDocument();
};

test("add 3 hours to deadline", async () => {
	const user = userEvent.setup();
	const updateDeadlineSpy = jest
		.spyOn(API, "putWorkspaceExtension")
		.mockResolvedValue();

	await renderScheduleControls();

	const addButton = screen.getByRole("button", {
		name: /add 1 hour to deadline/i,
	});
	await user.click(addButton);
	await user.click(addButton);
	await user.click(addButton);
	await screen.findByText(
		"Workspace shutdown time has been successfully updated.",
	);
	expect(
		await screen.findByText(/Stop in (about )?6 hours/),
	).toBeInTheDocument();

	// Mocks are used here because the 'usedDeadline' is a Date object, which
	// can't be directly compared with the same object.
	const usedWorkspaceId = updateDeadlineSpy.mock.calls[0][0];
	const usedDeadline = updateDeadlineSpy.mock.calls[0][1];
	expect(usedWorkspaceId).toEqual(MockWorkspace.id);
	expect(usedDeadline.toISOString()).toEqual(
		add(BASE_DEADLINE, { hours: 3 }).toISOString(),
	);
});

test("remove 2 hours to deadline", async () => {
	const user = userEvent.setup();
	const updateDeadlineSpy = jest
		.spyOn(API, "putWorkspaceExtension")
		.mockResolvedValue();

	await renderScheduleControls();

	const subButton = screen.getByRole("button", {
		name: /subtract 1 hour from deadline/i,
	});
	await user.click(subButton);
	await user.click(subButton);
	await screen.findByText(
		"Workspace shutdown time has been successfully updated.",
	);
	expect(
		await screen.findByText(/Stop in (about )?1 hour/),
	).toBeInTheDocument();

	// Mocks are used here because the 'usedDeadline' is a Date object, which
	// can't be directly compared with the same object.
	const usedWorkspaceId = updateDeadlineSpy.mock.calls[0][0];
	const usedDeadline = updateDeadlineSpy.mock.calls[0][1];
	expect(usedWorkspaceId).toEqual(MockWorkspace.id);
	expect(usedDeadline.toISOString()).toEqual(
		sub(BASE_DEADLINE, { hours: 2 }).toISOString(),
	);
});

test("rollback to previous deadline on error", async () => {
	const user = userEvent.setup();
	const initialScheduleMessage = /Stop in (about )?3 hours/;
	jest.spyOn(API, "putWorkspaceExtension").mockRejectedValue({});

	await renderScheduleControls();

	const addButton = screen.getByRole("button", {
		name: /add 1 hour to deadline/i,
	});
	await user.click(addButton);
	await user.click(addButton);
	await user.click(addButton);
	await screen.findByText(
		"We couldn't update your workspace shutdown time. Please try again.",
	);
	// In case of an error, the schedule message should remain unchanged
	expect(screen.getByText(initialScheduleMessage)).toBeInTheDocument();
});

test("request is only sent once when clicking multiple times", async () => {
	const user = userEvent.setup();
	const updateDeadlineSpy = jest
		.spyOn(API, "putWorkspaceExtension")
		.mockResolvedValue();

	await renderScheduleControls();

	const addButton = screen.getByRole("button", {
		name: /add 1 hour to deadline/i,
	});
	await user.click(addButton);
	await user.click(addButton);
	await user.click(addButton);
	await screen.findByText(
		"Workspace shutdown time has been successfully updated.",
	);
	expect(updateDeadlineSpy).toHaveBeenCalledTimes(1);
});
