import type { AuditLog } from "#/api/typesGenerated";
import { MockAuditLog } from "#/testHelpers/entities";
import { getAuditLogDescriptionOverride } from "./AuditLogDescription";

const chatAuditLog = (
	diff: AuditLog["diff"],
	overrides: Partial<AuditLog> = {},
): AuditLog => ({
	...MockAuditLog,
	resource_type: "chat",
	action: "write",
	status_code: 200,
	diff,
	...overrides,
});

describe("getAuditLogDescriptionOverride", () => {
	it("describes archived chat writes", () => {
		expect(
			getAuditLogDescriptionOverride(
				chatAuditLog({
					archived: { old: false, new: true, secret: false },
				}),
			),
		).toBe("{user} archived chat {target}");
	});

	it("describes unarchived chat writes", () => {
		expect(
			getAuditLogDescriptionOverride(
				chatAuditLog({
					archived: { old: true, new: false, secret: false },
				}),
			),
		).toBe("{user} unarchived chat {target}");
	});

	it("describes chat ACL writes as sharing updates", () => {
		const aclDiff = {
			old: {},
			new: {
				"3f46df8a-4b99-4941-b9f8-d9f243d68d91": {
					permissions: ["read"],
				},
			},
			secret: false,
		};

		expect(
			getAuditLogDescriptionOverride(chatAuditLog({ user_acl: aclDiff })),
		).toBe("{user} updated sharing for chat {target}");
		expect(
			getAuditLogDescriptionOverride(chatAuditLog({ group_acl: aclDiff })),
		).toBe("{user} updated sharing for chat {target}");
		expect(
			getAuditLogDescriptionOverride(
				chatAuditLog({ user_acl: aclDiff, group_acl: aclDiff }),
			),
		).toBe("{user} updated sharing for chat {target}");
	});

	it("does not override mixed chat writes", () => {
		expect(
			getAuditLogDescriptionOverride(
				chatAuditLog({
					archived: { old: false, new: true, secret: false },
					pin_order: { old: 1, new: 0, secret: false },
				}),
			),
		).toBeUndefined();
		expect(
			getAuditLogDescriptionOverride(
				chatAuditLog({
					user_acl: { old: {}, new: {}, secret: false },
					pin_order: { old: 1, new: 0, secret: false },
				}),
			),
		).toBeUndefined();
	});

	it("does not override failed or redirected writes", () => {
		const diff = { archived: { old: false, new: true, secret: false } };

		expect(
			getAuditLogDescriptionOverride(chatAuditLog(diff, { status_code: 400 })),
		).toBeUndefined();
		expect(
			getAuditLogDescriptionOverride(chatAuditLog(diff, { status_code: 303 })),
		).toBeUndefined();
	});

	it("does not override non-chat resources", () => {
		expect(
			getAuditLogDescriptionOverride(
				chatAuditLog(
					{ archived: { old: false, new: true, secret: false } },
					{ resource_type: "workspace" },
				),
			),
		).toBeUndefined();
	});
});
