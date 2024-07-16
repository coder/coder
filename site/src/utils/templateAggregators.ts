import type { Template, TemplateExample } from "api/typesGenerated";

export type StarterTemplatesByTag = Record<string, TemplateExample[]>;
export type TemplatesByOrg = Record<string, Template[]>;

export const getTemplatesByTag = (
  templates: TemplateExample[],
): StarterTemplatesByTag => {
  const tags: StarterTemplatesByTag = {
    all: templates,
  };

  for (const template of templates) {
    for (const tag of template.tags) {
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- this can be undefined
      if (tags[tag]) {
        tags[tag].push(template);
      } else {
        tags[tag] = [template];
      }
    };
  };

  return tags;
};

export const getTemplatesByOrg = (templates: Template[]): TemplatesByOrg => {
  const orgs: TemplatesByOrg = {
    all: templates,
  };

  for (const template of templates) {
    const org = template.organization_name;
    if (orgs[org]) {
      orgs[org].push(template);
    } else {
      orgs[org] = [template];
    }
  };

  return orgs;
};
