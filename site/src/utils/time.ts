export type TimeUnit = "days" | "hours";

export function humanDuration(durationInMs: number) {
  if (durationInMs === 0) {
    return "0 hours";
  }

  const timeUnit = suggestedTimeUnit(durationInMs);
  const durationValue =
    timeUnit === "days"
      ? durationInDays(durationInMs)
      : durationInHours(durationInMs);

  return `${durationValue} ${timeUnit}`;
}

export function suggestedTimeUnit(duration: number): TimeUnit {
  if (duration === 0) {
    return "hours";
  }

  return Number.isInteger(durationInDays(duration)) ? "days" : "hours";
}

export function durationInHours(duration: number): number {
  return duration / 1000 / 60 / 60;
}

export function durationInDays(duration: number): number {
  return duration / 1000 / 60 / 60 / 24;
}

export function formatTime(seconds: number): string {
  let value: {
    amount: number;
    unit: "seconds" | "minutes" | "hours";
  } = {
    amount: seconds,
    unit: "seconds",
  };

  if (seconds >= 60 && seconds < 3600) {
    value = {
      amount: Math.floor(seconds / 60),
      unit: "minutes",
    };
  } else {
    value = {
      amount: seconds / 3600,
      unit: "hours",
    };
  }

  if (value.amount === 1) {
    const singularUnit = value.unit.slice(0, -1);
    return `${value.amount} ${singularUnit}`;
  }

  return `${value.amount.toLocaleString(undefined, {
    maximumFractionDigits: 1,
    minimumFractionDigits: 0,
  })} ${value.unit}`;
}
