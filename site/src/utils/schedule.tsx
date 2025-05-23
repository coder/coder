import Link from "@mui/material/Link";
import type { Template, Workspace } from "api/typesGenerated";
import { HelpTooltipTitle } from "components/HelpTooltip/HelpTooltip";
import cronParser from "cron-parser";
import cronstrue from "cronstrue";
import {
	add,
	addDays,
	differenceInHours,
	differenceInMilliseconds,
	format,
	formatDistance,
	formatDuration,
	isAfter,
	isBefore,
	isSameDay,
	parseISO,
} from "date-fns";
import { utcToZonedTime } from "date-fns-tz";
import type { WorkspaceActivityStatus } from "modules/workspaces/activity";
import type { ReactNode } from "react";
import { Link as RouterLink } from "react-router-dom";
import { isWorkspaceOn } from "./workspace";
/**
 * @fileoverview Client-side counterpart of the coderd/autostart/schedule Go
 * package. This package is a variation on crontab that uses minute, hour and
 * day of week.
 */

/**
 * DEFAULT_TIMEZONE is the default timezone that crontab assumes unless one is
 * specified.
 */
const DEFAULT_TIMEZONE = "UTC";

/**
 * stripTimezone strips a leading timezone from a schedule string
 */
export const stripTimezone = (raw: string): string => {
	return raw.replace(/CRON_TZ=\S*\s/, "");
};

/**
 * extractTimezone returns a leading timezone from a schedule string if one is
 * specified; otherwise the specified defaultTZ
 */
export const extractTimezone = (
	raw: string,
	defaultTZ = DEFAULT_TIMEZONE,
): string => {
	const matches = raw.match(/CRON_TZ=\S*\s/g);

	if (matches && matches.length > 0) {
		return matches[0].replace(/CRON_TZ=/, "").trim();
	}
	return defaultTZ;
};

/** Language used in the schedule components */
const Language = {
	manual: "Manual",
	workspaceShuttingDownLabel: "Workspace is shutting down",
	afterStart: "after start",
	autostartLabel: "Starts at",
	autostopLabel: "Stops at",
};

export const autostartDisplay = (schedule: string | undefined): string => {
	if (schedule) {
		return (
			cronstrue
				.toString(stripTimezone(schedule), {
					throwExceptionOnParseError: false,
				})
				// We don't want to keep the At because it is on the label
				.replace("At", "")
		);
	}
	return Language.manual;
};

const isShuttingDown = (workspace: Workspace, deadline?: Date): boolean => {
	if (!deadline) {
		if (!workspace.latest_build.deadline) {
			return false;
		}
		deadline = workspace.latest_build.deadline
			? workspace.latest_build.deadline
				? parseISO(workspace.latest_build.deadline)
				: new Date()
			: new Date();
	}
	const now = new Date();
	return isWorkspaceOn(workspace) && isAfter(now, deadline);
};

export const autostopDisplay = (
	workspace: Workspace,
	activityStatus: WorkspaceActivityStatus,
	template: Template,
): {
	message: ReactNode;
	tooltip?: ReactNode;
	danger?: boolean;
} => {
	const ttl = workspace.ttl_ms;

	if (isWorkspaceOn(workspace) && workspace.latest_build.deadline) {
		// Workspace is on --> derive from latest_build.deadline. Note that the
		// user may modify their workspace object (ttl) while the workspace is
		// running and depending on system semantics, the deadline may still
		// represent the previously defined ttl. Thus, we always derive from the
		// deadline as the source of truth.

		const userTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
		const deadline = utcToZonedTime(
			workspace.latest_build.deadline
				? parseISO(workspace.latest_build.deadline)
				: new Date(),
			userTimezone,
		);
		const now = new Date();

		if (activityStatus === "connected") {
			const hasMaxDeadline = Boolean(workspace.latest_build.max_deadline);
			const maxDeadline = workspace.latest_build.max_deadline
				? workspace.latest_build.max_deadline
					? parseISO(workspace.latest_build.max_deadline)
					: new Date()
				: null;
			if (
				hasMaxDeadline &&
				maxDeadline &&
				isBefore(maxDeadline, add(now, { hours: 2 }))
			) {
				return {
					message: "Required to stop soon",
					tooltip: (
						<>
							<HelpTooltipTitle>Upcoming stop required</HelpTooltipTitle>
							This workspace will be required to stop by{" "}
							{format(
								workspace.latest_build.max_deadline
									? parseISO(workspace.latest_build.max_deadline)
									: new Date(),
								"MMMM d 'at' h:mm a",
							)}
							. You can restart your workspace before then to avoid
							interruption.
						</>
					),
					danger: true,
				};
			}
		}

		if (isShuttingDown(workspace, deadline)) {
			return {
				message: Language.workspaceShuttingDownLabel,
			};
		}
		let title = (
			<HelpTooltipTitle>Template Autostop requirement</HelpTooltipTitle>
		);
		let reason: ReactNode = ` because the ${template.display_name} template has an autostop requirement.`;
		if (template.autostop_requirement && template.allow_user_autostop) {
			title = <HelpTooltipTitle>Autostop schedule</HelpTooltipTitle>;
			reason = (
				<span data-chromatic="ignore">
					{" "}
					because this workspace has enabled autostop. You can disable autostop
					from this workspace&apos;s{" "}
					<Link component={RouterLink} to="settings/schedule">
						schedule settings
					</Link>
					.
				</span>
			);
		}
		return {
			message: `Stop ${formatDistance(deadline, now)}`,
			tooltip: (
				<span data-chromatic="ignore">
					{title}
					This workspace will be stopped on{" "}
					{format(deadline, "MMMM d 'at' h:mm a")}
					{reason}
				</span>
			),
			danger: isShutdownSoon(workspace),
		};
	}
	if (!ttl || ttl < 1) {
		// If the workspace is not on, and the ttl is 0 or undefined, then the
		// workspace is set to manually shutdown.
		return {
			message: Language.manual,
		};
	}
	// The workspace has a ttl set, but is either in an unknown state or is
	// not running. Therefore, we derive from workspace.ttl.
	return {
		message: `Stop ${formatDuration({
			hours: Math.floor(ttl / (1000 * 60 * 60)),
			minutes: Math.floor((ttl % (1000 * 60 * 60)) / (1000 * 60)),
		})} ${Language.afterStart}`,
	};
};

const isShutdownSoon = (workspace: Workspace): boolean => {
	const deadline = workspace.latest_build.deadline;
	if (!deadline) {
		return false;
	}
	const deadlineDate = parseISO(deadline);
	const now = new Date();
	const diff = differenceInMilliseconds(deadlineDate, now);
	const oneHour = 1000 * 60 * 60;
	return diff < oneHour;
};

// Define the extension durations
export const deadlineExtensionMin = 30 * 60 * 1000; // 30 minutes in milliseconds
export const deadlineExtensionMax = 24 * 60 * 60 * 1000; // 24 hours in milliseconds

/**
 * Depends on the time the workspace was last updated and a global constant.
 * @param ws workspace
 * @returns the latest datetime at which the workspace can be automatically shut down.
 */
export function getMaxDeadline(ws: Workspace | undefined): Date {
	// note: we count runtime from updated_at as started_at counts from the start of
	// the workspace build process, which can take a while.
	if (ws === undefined) {
		throw Error("Cannot calculate max deadline because workspace is undefined");
	}
	const startedAt = ws.latest_build.updated_at
		? parseISO(ws.latest_build.updated_at)
		: new Date();
	return add(startedAt, { hours: 24 });
}

/**
 * Depends on the current time and a global constant.
 * @returns the earliest datetime at which the workspace can be automatically shut down.
 */
export function getMinDeadline(): Date {
	return add(new Date(), { minutes: 30 });
}

export const getDeadline = (workspace: Workspace): Date =>
	workspace.latest_build.deadline
		? parseISO(workspace.latest_build.deadline)
		: new Date();

/**
 * Get number of hours you can add or subtract to the current deadline before hitting the max or min deadline.
 * @param deadline
 * @param workspace
 * @returns number, in hours
 */
export const getMaxDeadlineChange = (
	deadline: Date,
	extremeDeadline: Date,
): number => Math.abs(differenceInHours(deadline, extremeDeadline));

export const validTime = (time: string): boolean => {
	return /^[0-9][0-9]:[0-9][0-9]$/.test(time);
};

export const timeToCron = (time: string, tz?: string) => {
	if (!validTime(time)) {
		throw new Error(`Invalid time: ${time}`);
	}
	const [HH, mm] = time.split(":");
	let prefix = "";
	if (tz) {
		prefix = `CRON_TZ=${tz} `;
	}
	return `${prefix}${Number(mm)} ${Number(HH)} * * *`;
};

export const quietHoursDisplay = (
	browserLocale: string,
	time: string,
	tz: string,
	now: Date | undefined,
): string => {
	if (!validTime(time)) {
		return "Invalid time";
	}

	// The cron-parser package doesn't accept a timezone in the cron string, but
	// accepts it as an option.
	const cron = timeToCron(time);
	const parsed = cronParser.parseExpression(cron, {
		currentDate: now,
		iterator: false,
		utc: false,
		tz,
	});

	const today = utcToZonedTime(now || new Date(), tz);
	const day = utcToZonedTime(parsed.next().toDate(), tz);

	const formattedTime = new Intl.DateTimeFormat(browserLocale, {
		hour: "numeric",
		minute: "numeric",
		timeZone: tz,
	}).format(day);

	let display = formattedTime;

	if (isSameDay(day, today)) {
		display += " today";
	} else if (isSameDay(day, addDays(today, 1))) {
		display += " tomorrow";
	} else {
		// This case will rarely ever be hit, as we're dealing with only times and
		// not dates, but it can be hit due to mismatched browser timezone to cron
		// timezone or due to daylight savings changes.
		display += ` on ${format(day, "EEEE, MMMM d")}`;
	}

	display += ` (${formatDistance(day, today)}) in ${tz}`;

	return display;
};

export type TemplateAutostartRequirementDaysValue =
	| "monday"
	| "tuesday"
	| "wednesday"
	| "thursday"
	| "friday"
	| "saturday"
	| "sunday";

export type TemplateAutostopRequirementDaysValue =
	| "off"
	| "daily"
	| "saturday"
	| "sunday";

export const sortedDays = [
	"monday",
	"tuesday",
	"wednesday",
	"thursday",
	"friday",
	"saturday",
	"sunday",
] as TemplateAutostartRequirementDaysValue[];

export const calculateAutostopRequirementDaysValue = (
	value: TemplateAutostopRequirementDaysValue,
): Template["autostop_requirement"]["days_of_week"] => {
	switch (value) {
		case "daily":
			return [
				"monday",
				"tuesday",
				"wednesday",
				"thursday",
				"friday",
				"saturday",
				"sunday",
			];
		case "saturday":
			return ["saturday"];
		case "sunday":
			return ["sunday"];
	}

	return [];
};
