import type { ChatProjectTemplatesChange } from "#/api/queries/chatProjects";
import type {
	ChatProject,
	ChatProjectDefaultTemplate,
	Template,
} from "#/api/typesGenerated";
import { NIL_UUID } from "../../utils/modelOptions";
import {
	type ChatProjectTemplatesSettings,
	type ChatProjectTemplatesValues,
	chatProjectTemplatesRequest,
} from "../ChatsSidebar/dialogs/ChatProjectTemplatesFields";

/**
 * The project's saved template settings as the viewer sees them. Attached
 * templates the viewer cannot access are not in `template_ids`.
 */
export const savedTemplates = (
	project: ChatProject,
): ChatProjectTemplatesSettings => ({
	limit_templates: project.limit_templates,
	template_ids: project.templates.map(({ id }) => id),
	default_template: project.default_template,
});

const serializeDefaultTemplate = (
	template: ChatProjectDefaultTemplate | undefined,
) =>
	JSON.stringify([
		template?.template_id ?? "",
		template?.preset_id ?? "",
		Object.entries(template?.parameters ?? {}).sort(([a], [b]) =>
			a.localeCompare(b),
		),
	]);

/**
 * Serializes template settings so that saved and drafted ones compare by
 * content: the order of template IDs and of parameter keys is ignored.
 */
const serializeTemplates = (templates: ChatProjectTemplatesSettings) =>
	JSON.stringify([
		templates.limit_templates,
		templates.template_ids.toSorted(),
		serializeDefaultTemplate(templates.default_template),
	]);

/**
 * Whether `request` differs from what `base` already saves. Both leave out
 * templates that are no longer eligible, so a draft that only selects one of
 * those has nothing to save. False while the eligible templates load.
 */
export const hasTemplatesChanges = (
	request: ChatProjectTemplatesSettings | undefined,
	base: ChatProjectTemplatesValues,
	eligible: readonly Template[] | undefined,
) =>
	request !== undefined &&
	eligible !== undefined &&
	serializeTemplates(request) !==
		serializeTemplates(chatProjectTemplatesRequest(base, eligible));

/**
 * Returns what saving the draft changes about the saved templates. Templates
 * that are in neither are left alone, which covers the ones the user cannot
 * see.
 */
export const templatesChange = (
	saved: ChatProjectTemplatesSettings,
	draft: ChatProjectTemplatesSettings,
): ChatProjectTemplatesChange => {
	const limitChanged = draft.limit_templates !== saved.limit_templates;
	const defaultChanged =
		serializeDefaultTemplate(draft.default_template) !==
		serializeDefaultTemplate(saved.default_template);
	const request = {
		...(limitChanged && { limit_templates: draft.limit_templates }),
		...(defaultChanged && {
			default_template: draft.default_template ?? { template_id: NIL_UUID },
		}),
	};
	return {
		attach: draft.template_ids.filter((id) => !saved.template_ids.includes(id)),
		detach: saved.template_ids.filter((id) => !draft.template_ids.includes(id)),
		...((limitChanged || defaultChanged) && { request }),
	};
};
