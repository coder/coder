import tzData from "tzdata";

export const timeZones = Object.keys(tzData.zones).sort();

export const getPreferredTimezone = () =>
  Intl.DateTimeFormat().resolvedOptions().timeZone;
