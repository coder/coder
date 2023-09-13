import tzData from "tzdata";

export const timeZones = Object.keys(tzData.zones).sort();
