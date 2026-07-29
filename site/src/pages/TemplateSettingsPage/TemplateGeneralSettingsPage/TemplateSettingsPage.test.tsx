import type { UpdateTemplateMeta } from "#/api/typesGenerated";
import { validationSchema } from "./TemplateSettingsForm";

type FormValues = Required<
	Omit<
		UpdateTemplateMeta,
		| "default_ttl_ms"
		| "activity_bump_ms"
		| "time_til_autostop_notify_ms"
		| "deprecation_message"
	>
>;

const validFormValues: FormValues = {
	name: "Name",
	display_name: "A display name",
	description: "A description",
	icon: "vscode.png",
	allow_user_cancel_workspace_jobs: false,
	allow_user_autostart: false,
	allow_user_autostop: false,
	autostop_requirement: {
		days_of_week: [],
		weeks: 1,
	},
	autostart_requirement: {
		days_of_week: [
			"monday",
			"tuesday",
			"wednesday",
			"thursday",
			"friday",
			"saturday",
			"sunday",
		],
	},
	failure_ttl_ms: 0,
	time_til_dormant_ms: 0,
	time_til_dormant_autodelete_ms: 0,
	update_workspace_last_used_at: false,
	update_workspace_dormant_at: false,
	require_active_version: false,
	disable_everyone_group_access: false,
	max_port_share_level: "owner",
	use_classic_parameter_flow: true,
	cors_behavior: "simple",
	disable_module_cache: false,
};

describe("TemplateSettingsPage", () => {
	it("allows a description of 128 chars", () => {
		const values: UpdateTemplateMeta = {
			...validFormValues,
			description:
				"The quick brown fox jumps over the lazy dog repeatedly, enjoying the weather of the bright, summer day in the lush, scenic park.",
		};
		const validate = () => validationSchema.validateSync(values);
		expect(validate).not.toThrowError();
	});

	it("disallows a description of 128 + 1 chars", () => {
		const values: UpdateTemplateMeta = {
			...validFormValues,
			description:
				"The quick brown fox jumps over the lazy dog multiple times, enjoying the warmth of the bright, sunny day in the lush, green park.",
		};
		const validate = () => validationSchema.validateSync(values);
		expect(validate).toThrowError();
	});
});
