import type { TemplateExample } from "api/typesGenerated";

export type StarterTemplatesByTag = Record<string, TemplateExample[]>;

export const getTemplatesByTag = (
	templates: TemplateExample[],
): StarterTemplatesByTag => {
	const tags: StarterTemplatesByTag = {
		all: templates,
	};

	for (const template of templates) {
		for (const tag of template.tags) {
			if (!tags[tag]) {
				tags[tag] = [];
			}
			tags[tag].push(template);
		}
	}

	return tags;
};
