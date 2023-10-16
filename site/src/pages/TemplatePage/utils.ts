import { Template } from "api/typesGenerated";
import { pageTitle } from "utils/page";

export const getTemplatePageTitle = (title: string, template: Template) => {
  return pageTitle(
    `${
      template.display_name.length > 0 ? template.display_name : template.name
    } · ${title}`,
  );
};
