/**
 * Ideally the version of tzdata should correspond to the version of the
 * timezone database used by the version of Node we're running our tests
 * against. For example, Node v20.19.4 and tzdata@1.0.44 both correspond to
 * version 2025b of the ICU timezone:
 * https://github.com/nodejs/node/blob/v20.19.4/test/fixtures/tz-version.txt
 * https://github.com/rogierschouten/tzdata-generate/releases/tag/v1.0.44
 *
 * For some reason though, the timezones allowed by `Intl.DateTimeFormat` in
 * Node diverged slightly from the timezones present in the tzdata package,
 * despite being derived from the same data. Notably, the timezones that we
 * filter out below are not allowed by Node as of v20.18.1 and onward–which is
 * the version that updated the 20 release line from 2024a to 2024b.
 */
import tzData from "tzdata";

export const timeZones = Object.keys(tzData.zones)
	.filter((it) => it !== "Factory" && it !== "null")
	.sort();

export const getPreferredTimezone = () =>
	Intl.DateTimeFormat().resolvedOptions().timeZone;
