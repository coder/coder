import type { ChatProjectTemplatesChange } from "#/api/queries/chatProjects";
import type {
	ChatProject,
	ChatProjectDefaultTemplate,
} from "#/api/typesGenerated";
import { NIL_UUID } from "../../utils/modelOptions";
import type {
	ChatProjectTemplatesSettings,
	ChatProjectTemplatesValues,
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

/** Serializes form values, to tell whether the user changed the draft. */
export const serializeValues = (values: ChatProjectTemplatesValues) =>
	JSON.stringify({
		...values,
		templateIds: values.templateIds.toSorted(),
		defaultParameters: Object.entries(values.defaultParameters).sort(
			([a], [b]) => a.localeCompare(b),
		),
	});
