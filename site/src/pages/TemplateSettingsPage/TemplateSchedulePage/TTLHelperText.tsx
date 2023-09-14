const hours = (h: number) => (h === 1 ? "hour" : "hours");
const days = (d: number) => (d === 1 ? "day" : "days");

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
      Workspaces will default to stopping after {ttl} {hours(ttl)} without
      activity.
    </span>
  );
};

export const MaxTTLHelperText = (props: { ttl?: number }) => {
  const { ttl = 0 } = props;

  // Error will show once field is considered touched
  if (ttl < 0) {
    return null;
  }

  if (ttl === 0) {
    return <span>Workspaces may run indefinitely.</span>;
  }

  return (
    <span>
      Workspaces must stop within {ttl} {hours(ttl)} of starting, regardless of
      any active connections.
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
      Coder will attempt to stop failed workspaces after {ttl} {days(ttl)}.
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
      Coder will mark workspaces as dormant after {ttl} {days(ttl)} without user
      connections.
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
      Coder will automatically delete dormant workspaces after {ttl} {days(ttl)}
      .
    </span>
  );
};
