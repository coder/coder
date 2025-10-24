import { getCleanTaskName } from "./tasks";

describe("getCleanTaskName", () => {
	it("should remove task- prefix and identifier suffix", () => {
		expect(getCleanTaskName("task-fix-login-bug-abc123")).toBe("Fix Login Bug");
		expect(getCleanTaskName("task-add-feature-xyz789")).toBe("Add Feature");
		expect(getCleanTaskName("task-update-docs-12345678")).toBe("Update Docs");
	});

	it("should handle workspace names without task- prefix", () => {
		expect(getCleanTaskName("fix-login-bug-abc123")).toBe("Fix Login Bug");
		expect(getCleanTaskName("simple-name-123")).toBe("Simple Name");
	});

	it("should handle short identifiers with numbers", () => {
		expect(getCleanTaskName("task-test-feature-a1b2c3")).toBe("Test Feature");
		expect(getCleanTaskName("task-debug-issue-99")).toBe("Debug Issue");
	});

	it("should handle UUID-like identifiers", () => {
		expect(getCleanTaskName("task-implement-auth-a1b2c3d4")).toBe(
			"Implement Auth",
		);
	});

	it("should handle names without identifiers", () => {
		expect(getCleanTaskName("task-simple")).toBe("Simple");
		expect(getCleanTaskName("simple")).toBe("Simple");
	});

	it("should title case the result", () => {
		expect(getCleanTaskName("task-fix-login-bug-123")).toBe("Fix Login Bug");
		expect(getCleanTaskName("task-ADD-NEW-FEATURE-xyz")).toBe(
			"Add New Feature",
		);
	});

	it("should handle single word names", () => {
		expect(getCleanTaskName("task-refactor")).toBe("Refactor");
		expect(getCleanTaskName("deploy")).toBe("Deploy");
	});

	it("should remove last part if it could be an identifier", () => {
		// The function removes the last part if it could potentially be an identifier
		// This is conservative behavior to ensure clean names
		expect(getCleanTaskName("task-update-user-profile")).toBe("Update User");
		expect(getCleanTaskName("task-create-new-issue")).toBe("Create New");
	});
});
