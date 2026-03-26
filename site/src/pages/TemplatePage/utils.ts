import { pageTitle } from "utils/page";
import type { Template } from "#/api/typesGenerated";

export const getTemplatePageTitle = (title: string, template: Template) => {
	return pageTitle(
		`${
			template.display_name.length > 0 ? template.display_name : template.name
		} · ${title}`,
	);
};
