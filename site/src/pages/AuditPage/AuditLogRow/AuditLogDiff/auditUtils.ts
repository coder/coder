import type { AuditDiff } from "#/api/typesGenerated";

interface GroupMember {
	user_id: string;
	group_id: string;
}

/**
 *
 * @param auditLogDiff
 * @returns a diff with the 'members' key flattened to be an array of user_ids
 */
export const determineGroupDiff = (auditLogDiff: AuditDiff): AuditDiff => {
	const old = auditLogDiff.members?.old as GroupMember[] | undefined;
	const new_ = auditLogDiff.members?.new as GroupMember[] | undefined;

	return {
		...auditLogDiff,
		members: {
			old: old?.map((groupMember) => groupMember.user_id),
			new: new_?.map((groupMember) => groupMember.user_id),
			secret: auditLogDiff.members?.secret,
		},
	};
};

/**
 * Formats an audit diff value for display. Strings are quoted, nullish values
 * become "null", SQL time objects are localized, arrays are recursed, and plain
 * objects are serialized as compact JSON with sorted keys.
 */
export const formatAuditDiffValue = (value: unknown): string => {
	if (typeof value === "string") {
		return JSON.stringify(value);
	}

	if (isTimeObject(value)) {
		if (!value.Valid) {
			return "null";
		}

		return new Date(value.Time).toLocaleString();
	}

	if (Array.isArray(value)) {
		const values = value.map((v) => formatAuditDiffValue(v));
		return `[${values.join(", ")}]`;
	}

	if (value === null || value === undefined) {
		return "null";
	}

	if (isPlainObject(value)) {
		return JSON.stringify(sortObjectKeys(value));
	}

	return String(value);
};

const isTimeObject = (
	value: unknown,
): value is { Time: string; Valid: boolean } => {
	return (
		value !== null &&
		typeof value === "object" &&
		"Time" in value &&
		typeof value.Time === "string" &&
		"Valid" in value &&
		typeof value.Valid === "boolean"
	);
};

const isPlainObject = (value: unknown): value is Record<string, unknown> => {
	return Object.prototype.toString.call(value) === "[object Object]";
};

const sortObjectKeys = (value: unknown): unknown => {
	if (Array.isArray(value)) {
		return value.map(sortObjectKeys);
	}

	if (!isPlainObject(value)) {
		return value;
	}

	const sorted: Record<string, unknown> = {};
	for (const key of Object.keys(value).sort()) {
		sorted[key] = sortObjectKeys(value[key]);
	}
	return sorted;
};
