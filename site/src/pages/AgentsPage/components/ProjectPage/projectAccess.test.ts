import { describe, expect, it } from "vitest";
import type { ChatProjectACL } from "#/api/typesGenerated";
import {
	MockGroup,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { buildAccessEntries, formatMembersHeading } from "./projectAccess";

describe("formatMembersHeading", () => {
	it.each([
		[1, 0, "1 member"],
		[3, 0, "3 members"],
		[1, 1, "1 member, 1 group"],
		[2, 3, "2 members, 3 groups"],
	])("formats %i members and %i groups as %s", (members, groups, expected) => {
		expect(formatMembersHeading(members, groups)).toBe(expected);
	});
});

describe("buildAccessEntries", () => {
	const mockUnnamedUser = {
		...MockUserMember,
		id: "unnamed-user",
		username: "unnamed",
		name: "",
	};
	const mockACL: ChatProjectACL = {
		users: [
			{ ...MockUserMember, role: "admin" },
			{ ...mockUnnamedUser, role: "use" },
		],
		groups: [{ ...MockGroup, role: "use" }],
	};

	it("lists the owner first, then users, then groups", () => {
		expect(
			buildAccessEntries(MockUserOwner.id, MockUserOwner, mockACL).map(
				({ kind, id }) => ({ kind, id }),
			),
		).toEqual([
			{ kind: "owner", id: MockUserOwner.id },
			{ kind: "user", id: MockUserMember.id },
			{ kind: "user", id: mockUnnamedUser.id },
			{ kind: "group", id: MockGroup.id },
		]);
	});

	it("names users by username when they have no display name", () => {
		const [, , unnamed] = buildAccessEntries(
			MockUserOwner.id,
			MockUserOwner,
			mockACL,
		);
		expect(unnamed).toMatchObject({
			name: mockUnnamedUser.username,
			subtitle: undefined,
		});
	});

	it.each([
		["loading", undefined, undefined],
		["failed to load", null, "Unknown"],
	])("names the owner when it is %s", (_, owner, name) => {
		const [ownerEntry] = buildAccessEntries(MockUserOwner.id, owner, mockACL);
		expect(ownerEntry).toMatchObject({ kind: "owner", name });
	});
});
