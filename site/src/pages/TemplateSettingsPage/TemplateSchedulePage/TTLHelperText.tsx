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
}) => {
	const { bump = 0, defaultTTL = 0 } = props;

	if (!defaultTTL) {
		return (
			<span>
				Activity bump only applies when a default TTL is configured. Set a
				default TTL above to enable activity bumping.
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
}) => {
	const { lead = 0, defaultTTL = 0, autostopRequirementDaysOfWeek } = props;

	const hasAutostopRequirement =
		Boolean(autostopRequirementDaysOfWeek) &&
		autostopRequirementDaysOfWeek !== "off";

	if (!defaultTTL && !hasAutostopRequirement) {
		return (
			<span>
				Autostop reminders only apply when an autostop deadline is configured.
				Set a default TTL or an autostop requirement above to enable reminders.
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
