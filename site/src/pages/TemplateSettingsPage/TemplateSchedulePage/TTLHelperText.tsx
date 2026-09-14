import { humanDuration } from "#/utils/time";

const hours = (h: number) => (h === 1 ? "hour" : "hours");

export const DefaultTTLHelperText = (props: { ttl?: number }) => {
	const { ttl = 0 } = props;

	// Error will show once field is considered touched
	if (ttl < 0) {
		return null;
	}

	if (ttl === 0) {
		return <span>Workspaces will run until stopped manually.</span>;
	}

	return (
		<span>
			Workspaces will default to stopping after {ttl} {hours(ttl)} after being
			started.
		</span>
	);
};

export const ActivityBumpHelperText = (props: {
	bump?: number;
	defaultTTL?: number;
	allowUserAutostop?: boolean;
}) => {
	const { bump = 0, defaultTTL = 0, allowUserAutostop = false } = props;

	// Activity bump extends a workspace's scheduled stop time. If there is no
	// default TTL AND users cannot set their own autostop, there is no stop
	// time to bump, so the field has no effect.
	if (!defaultTTL && !allowUserAutostop) {
		return (
			<span>
				Activity bump only applies when "Default autostop" is configured or
				users are allowed to customize autostop. Set "Default autostop" above or
				check "Allow users to customize autostop duration for workspaces" below
				to enable activity bumping.
			</span>
		);
	}

	// Error will show once field is considered touched
	if (bump < 0) {
		return null;
	}

	if (bump === 0) {
		return (
			<span>
				Workspaces will not have their stop time automatically extended based on
				user activity. Users can still manually delay the stop time.
			</span>
		);
	}

	return (
		<span>
			Workspaces will be automatically bumped by {bump} {hours(bump)} when user
			activity is detected.
		</span>
	);
};

export const AutostopReminderHelperText = (props: {
	lead?: number;
	defaultTTL?: number;
	autostopRequirementDaysOfWeek?: string;
	allowUserAutostop?: boolean;
}) => {
	const {
		lead = 0,
		defaultTTL = 0,
		autostopRequirementDaysOfWeek,
		allowUserAutostop = false,
	} = props;

	const hasAutostopRequirement =
		Boolean(autostopRequirementDaysOfWeek) &&
		autostopRequirementDaysOfWeek !== "off";

	// Autostop reminders fire relative to a workspace's scheduled stop, so
	// this hint only makes sense when none of the sources of a stop deadline
	// (default TTL, autostop requirement, or user-configured autostop) are
	// available.
	if (!defaultTTL && !hasAutostopRequirement && !allowUserAutostop) {
		return (
			<span>
				Autostop reminders only apply when an autostop deadline is configured.
				Set "Default autostop", an autostop requirement, or check "Allow users
				to customize autostop duration for workspaces" to enable reminders.
			</span>
		);
	}

	// Error will show once field is considered touched
	if (lead < 0) {
		return null;
	}

	if (lead === 0) {
		return (
			<span>
				Workspace owners will not be reminded before their workspace is
				automatically stopped.
			</span>
		);
	}

	return (
		<span>
			Workspace owners will be reminded {lead} {hours(lead)} before their
			workspace is automatically stopped.
		</span>
	);
};

export const FailureTTLHelperText = (props: { ttl?: number }) => {
	const { ttl = 0 } = props;

	// Error will show once field is considered touched
	if (ttl < 0) {
		return null;
	}

	if (ttl === 0) {
		return <span>Coder will not automatically stop failed workspaces.</span>;
	}

	return (
		<span>
			Coder will attempt to stop failed workspaces after {humanDuration(ttl)}.
		</span>
	);
};

export const DormancyTTLHelperText = (props: { ttl?: number }) => {
	const { ttl = 0 } = props;

	// Error will show once field is considered touched
	if (ttl < 0) {
		return null;
	}

	if (ttl === 0) {
		return <span>Coder will not mark workspaces as dormant.</span>;
	}

	return (
		<span>
			Coder will mark workspaces as dormant after {humanDuration(ttl)} without
			user connections.
		</span>
	);
};

export const DormancyAutoDeletionTTLHelperText = (props: { ttl?: number }) => {
	const { ttl = 0 } = props;

	// Error will show once field is considered touched
	if (ttl < 0) {
		return null;
	}

	if (ttl === 0) {
		return <span>Coder will not automatically delete dormant workspaces.</span>;
	}

	return (
		<span>
			Coder will automatically delete dormant workspaces after{" "}
			{humanDuration(ttl)}.
		</span>
	);
};
