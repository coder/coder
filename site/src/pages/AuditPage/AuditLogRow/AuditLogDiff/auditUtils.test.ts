import { determineGroupDiff, formatAuditDiffValue } from "./auditUtils";

const auditDiffForNewGroup = {
	id: {
		old: "",
		new: "e22e0eb9-625a-468b-b962-269b19473789",
		secret: false,
	},
	members: {
		new: [],
		secret: false,
	},
	name: {
		old: "",
		new: "another-test-group",
		secret: false,
	},
};

const auditDiffForAddedGroupMember = {
	members: {
		old: [],
		new: [
			{
				group_id: "e22e0eb9-625a-468b-b962-269b19473789",
				user_id: "cea4c2b0-6373-4858-b26a-df3cbfce8845",
			},
		],
		secret: false,
	},
};

const auditDiffForRemovedGroupMember = {
	members: {
		old: [
			{
				group_id: "25793395-b093-4a3c-a473-9ecf9b243478",
				user_id: "84d1cd5a-17e1-4022-898c-52e64256e737",
			},
			{
				group_id: "25793395-b093-4a3c-a473-9ecf9b243478",
				user_id: "cea4c2b0-6373-4858-b26a-df3cbfce8845",
			},
		],
		new: [
			{
				group_id: "25793395-b093-4a3c-a473-9ecf9b243478",
				user_id: "84d1cd5a-17e1-4022-898c-52e64256e737",
			},
		],
		secret: false,
	},
};

const AuditDiffForDeletedGroup = {
	id: {
		old: "25793395-b093-4a3c-a473-9ecf9b243478",
		new: "",
		secret: false,
	},
	members: {
		old: [
			{
				group_id: "25793395-b093-4a3c-a473-9ecf9b243478",
				user_id: "84d1cd5a-17e1-4022-898c-52e64256e737",
			},
		],
		secret: false,
	},
	name: {
		old: "test-group",
		new: "",
		secret: false,
	},
};

describe("determineAuditDiff", () => {
	it("auditDiffForNewGroup", () => {
		// there should be no change as members are not added when a group is created
		expect(determineGroupDiff(auditDiffForNewGroup)).toEqual(
			auditDiffForNewGroup,
		);
	});

	it("auditDiffForAddedGroupMember", () => {
		const result = {
			members: {
				...auditDiffForAddedGroupMember.members,
				new: ["cea4c2b0-6373-4858-b26a-df3cbfce8845"],
			},
		};

		expect(determineGroupDiff(auditDiffForAddedGroupMember)).toEqual(result);
	});

	it("auditDiffForRemovedGroupMember", () => {
		const result = {
			members: {
				...auditDiffForRemovedGroupMember.members,
				old: [
					"84d1cd5a-17e1-4022-898c-52e64256e737",
					"cea4c2b0-6373-4858-b26a-df3cbfce8845",
				],
				new: ["84d1cd5a-17e1-4022-898c-52e64256e737"],
			},
		};

		expect(determineGroupDiff(auditDiffForRemovedGroupMember)).toEqual(result);
	});

	it("AuditDiffForDeletedGroup", () => {
		const result = {
			...AuditDiffForDeletedGroup,
			members: {
				...AuditDiffForDeletedGroup.members,
				old: ["84d1cd5a-17e1-4022-898c-52e64256e737"],
			},
		};

		expect(determineGroupDiff(AuditDiffForDeletedGroup)).toEqual(result);
	});
});

describe("formatAuditDiffValue", () => {
	it.each([
		{ name: "string", value: "hello", expected: '"hello"' },
		{
			name: "string containing double quotes",
			value: 'he said "hello"',
			expected: '"he said \\"hello\\""',
		},
		{
			name: "array of primitives",
			value: ["admin", "auditor"],
			expected: '["admin", "auditor"]',
		},
		{ name: "boolean true", value: true, expected: "true" },
		{ name: "boolean false", value: false, expected: "false" },
		{ name: "number", value: 42, expected: "42" },
		{ name: "null", value: null, expected: "null" },
		{ name: "undefined", value: undefined, expected: "null" },
		{
			name: "invalid SQL time",
			value: { Time: "0001-01-01T00:00:00Z", Valid: false },
			expected: "null",
		},
	])("preserves current behavior for $name", ({ value, expected }) => {
		expect(formatAuditDiffValue(value)).toBe(expected);
	});

	it("preserves current behavior for valid SQL time objects", () => {
		const value = { Time: "2024-10-22T09:03:23.961702Z", Valid: true };

		expect(formatAuditDiffValue(value)).toBe(
			new Date(value.Time).toLocaleString(),
		);
	});

	it("formats plain objects as deterministic compact JSON", () => {
		expect(
			formatAuditDiffValue({
				z: ["read"],
				a: { permissions: ["read"] },
			}),
		).toBe('{"a":{"permissions":["read"]},"z":["read"]}');
	});

	it("formats chat ACL objects as deterministic compact JSON", () => {
		expect(
			formatAuditDiffValue({
				"user-2": { permissions: ["read"] },
				"user-1": { permissions: ["read"] },
			}),
		).toBe(
			'{"user-1":{"permissions":["read"]},"user-2":{"permissions":["read"]}}',
		);
	});

	it("formats arrays containing objects without object string coercion", () => {
		expect(
			formatAuditDiffValue([
				{ user_id: "user-2", permissions: ["read"] },
				{ permissions: ["read"], user_id: "user-1" },
			]),
		).toBe(
			'[{"permissions":["read"],"user_id":"user-2"}, {"permissions":["read"],"user_id":"user-1"}]',
		);
	});
});
