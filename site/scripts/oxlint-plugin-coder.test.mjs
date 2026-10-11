import { RuleTester } from "oxlint/plugins-dev";
import plugin from "./oxlint-plugin-coder.mjs";

const ruleTester = new RuleTester({
	languageOptions: {
		sourceType: "module",
		parserOptions: { lang: "ts" },
	},
});

ruleTester.run(
	"no-empty-catch-callback",
	plugin.rules["no-empty-catch-callback"],
	{
		valid: [
			"promise.catch((error) => console.error(error));",
			"promise.catch(() => {\n\t// The caller renders the error.\n});",
			"promise.catch(handleError);",
			"promise.then(() => {});",
		],
		invalid: [
			{
				code: "promise.catch(() => {});",
				errors: [{ messageId: "emptyCatchCallback" }],
			},
			{
				code: "promise.catch(async () => {});",
				errors: [{ messageId: "emptyCatchCallback" }],
			},
			{
				code: "promise.catch(function () {});",
				errors: [{ messageId: "emptyCatchCallback" }],
			},
		],
	},
);

ruleTester.run("no-as-unknown-as", plugin.rules["no-as-unknown-as"], {
	valid: ["const n = value as number;", "const u = value as unknown;"],
	invalid: [
		{
			code: "const n = value as unknown as number;",
			errors: [{ messageId: "asUnknownAs" }],
		},
	],
});

ruleTester.run(
	"require-disable-reason",
	plugin.rules["require-disable-reason"],
	{
		valid: [
			"// oxlint-disable-next-line no-debugger -- Exercises the debugger.\ndebugger;",
			"/* eslint-disable no-console -- CLI output. */",
			"// oxlint-enable no-debugger",
			"// Explains why oxlint-disable-next-line is avoided here.",
		],
		invalid: [
			{
				code: "// oxlint-disable-next-line no-debugger\ndebugger;",
				errors: [{ messageId: "missingReason", line: 1 }],
			},
			{
				code: "debugger; // eslint-disable-line no-debugger",
				errors: [{ messageId: "missingReason" }],
			},
			{
				code: "/* oxlint-disable */",
				errors: [{ messageId: "missingReason" }],
			},
			{
				code: "// oxlint-disable-next-line no-debugger --\ndebugger;",
				errors: [{ messageId: "missingReason" }],
			},
		],
	},
);
