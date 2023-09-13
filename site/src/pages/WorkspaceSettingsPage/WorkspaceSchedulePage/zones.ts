import tzData from "tzdata";

export const zones: string[] = Object.keys(tzData.zones).sort();
