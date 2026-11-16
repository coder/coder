import type { ChatProjectTemplatesChange } from "#/api/queries/chatProjects";
import type {
	ChatProject,
	ChatProjectDefaultTemplate,
} from "#/api/typesGenerated";
import { NIL_UUID } from "../../utils/modelOptions";
import {
	type ChatProjectTemplatesSettings,
	type ChatProjectTemplatesValues,
	chatProjectTemplatesValues,
} from "../ChatsSidebar/dialogs/ChatProjectTemplatesFields";

/** The project's saved templates, in the shape of a draft's request. */
export const savedTemplates = (
	project: ChatProject,
): ChatProjectTemplatesSettings => ({
	limit_templates: project.limit_templates,
	template_ids: project.templates.map(({ id }) => id),
	default_template: project.default_template,
});

/**
 * Draft and server copies can list parameter keys in different orders, so
 * keys are sorted before comparing.
 */
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
export const serializeTemplates = (templates: ChatProjectTemplatesSettings) =>
	JSON.stringify([
		templates.limit_templates,
		templates.template_ids.toSorted(),
		serializeDefaultTemplate(templates.default_template),
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

/** Serializes only the default template and its default values. */
const serializeDefaults = (values: ChatProjectTemplatesValues) =>
	serializeValues({ ...values, limitTemplates: false, templateIds: [] });

/**
 * Applies the draft's changes since `base` to `latest`, so saving the result
 * never undoes a template that someone else attached or detached. Where both
 * changed the limit or the default template, the draft wins.
 */
export const rebaseValues = (
	draft: ChatProjectTemplatesValues,
	base: ChatProjectTemplatesValues,
	latest: ChatProjectTemplatesValues,
): ChatProjectTemplatesValues => {
	const templateIds = [
		...latest.templateIds.filter(
			(id) => draft.templateIds.includes(id) || !base.templateIds.includes(id),
		),
		...draft.templateIds.filter(
			(id) =>
				!base.templateIds.includes(id) && !latest.templateIds.includes(id),
		),
	];
	// The default template is saved as a whole, so the draft's wins when the
	// user changed any part of it.
	const defaults =
		serializeDefaults(draft) === serializeDefaults(base) ? latest : draft;
	return {
		...(templateIds.includes(defaults.defaultTemplateId)
			? defaults
			: chatProjectTemplatesValues(undefined)),
		limitTemplates:
			draft.limitTemplates === base.limitTemplates
				? latest.limitTemplates
				: draft.limitTemplates,
		templateIds,
	};
};
