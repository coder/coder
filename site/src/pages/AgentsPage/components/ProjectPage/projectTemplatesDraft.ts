import type { ChatProjectTemplatesChange } from "#/api/queries/chatProjects";
import type {
	ChatProject,
	ChatProjectDefaultTemplate,
} from "#/api/typesGenerated";
import { NIL_UUID } from "../../utils/modelOptions";
import {
	type ChatProjectTemplatesRequest,
	type ChatProjectTemplatesValues,
	chatProjectTemplatesValues,
} from "../ChatsSidebar/dialogs/ChatProjectTemplatesFields";

/** The project's saved templates, in the shape of a draft's request. */
export const savedTemplates = (
	project: ChatProject,
): ChatProjectTemplatesRequest => ({
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

/** Serializes saved or drafted templates for comparison. */
export const serializeTemplates = (templates: ChatProjectTemplatesRequest) =>
	JSON.stringify([
		templates.limit_templates,
		[...templates.template_ids].sort(),
		serializeDefaultTemplate(templates.default_template),
	]);

/**
 * Returns what saving the draft changes about the templates it started
 * from. Only the user's own edits are saved, so a save never undoes a
 * template attached elsewhere, including the ones this user cannot see.
 */
export const templatesChange = (
	base: ChatProjectTemplatesRequest,
	draft: ChatProjectTemplatesRequest,
): ChatProjectTemplatesChange => {
	const limitChanged = draft.limit_templates !== base.limit_templates;
	const defaultChanged =
		serializeDefaultTemplate(draft.default_template) !==
		serializeDefaultTemplate(base.default_template);
	return {
		attach: draft.template_ids.filter((id) => !base.template_ids.includes(id)),
		detach: base.template_ids.filter((id) => !draft.template_ids.includes(id)),
		...((limitChanged || defaultChanged) && {
			request: {
				...(limitChanged && { limit_templates: draft.limit_templates }),
				...(defaultChanged && {
					default_template: draft.default_template ?? {
						template_id: NIL_UUID,
					},
				}),
			},
		}),
	};
};

/** Serializes form values, to tell whether the user changed the draft. */
export const serializeValues = (values: ChatProjectTemplatesValues) =>
	JSON.stringify({
		...values,
		templateIds: [...values.templateIds].sort(),
		defaultParameters: Object.entries(values.defaultParameters).sort(
			([a], [b]) => a.localeCompare(b),
		),
	});

/**
 * Applies the draft's changes since `base` to `latest`. The result keeps the
 * user's edits and everything saved elsewhere, so saving it never undoes a
 * template that someone else attached or detached.
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
	const serializeDefaults = (values: ChatProjectTemplatesValues) =>
		serializeValues({ ...values, limitTemplates: false, templateIds: [] });
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
