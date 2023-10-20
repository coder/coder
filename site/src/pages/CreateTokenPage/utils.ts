export const NANO_HOUR = 3600000000000;

export interface CreateTokenData {
  name: string;
  lifetime: number;
}

export interface LifetimeDay {
  label: string;
  value: number | string;
}

export const lifetimeDayPresets: LifetimeDay[] = [
  {
    label: "7 days",
    value: 7,
  },
  {
    label: "30 days",
    value: 30,
  },
  {
    label: "60 days",
    value: 60,
  },
  {
    label: "90 days",
    value: 90,
  },
];

export const customLifetimeDay: LifetimeDay = {
  label: "Custom",
  value: "custom",
};

export const filterByMaxTokenLifetime = (
  maxTokenLifetime?: number,
): LifetimeDay[] => {
  // if maxTokenLifetime hasn't been set, return the full array of options
  if (!maxTokenLifetime) {
    return lifetimeDayPresets;
  }

  // otherwise only return options that are less than or equal to the max lifetime
  return lifetimeDayPresets.filter(
    (lifetime) => Number(lifetime.value) <= maxTokenLifetime / NANO_HOUR / 24,
  );
};

export const determineDefaultLtValue = (
  maxTokenLifetime?: number,
): string | number => {
  const filteredArr = filterByMaxTokenLifetime(maxTokenLifetime);

  // default to a lifetime of 30 days if within the maxTokenLifetime
  const thirtyDayDefault = filteredArr.find((lt) => lt.value === 30);
  if (thirtyDayDefault) {
    return thirtyDayDefault.value;
  }

  // otherwise default to the first preset option
  if (filteredArr[0]) {
    return filteredArr[0].value;
  }

  // if no preset options are within the maxTokenLifetime, default to "custom"
  return "custom";
};
