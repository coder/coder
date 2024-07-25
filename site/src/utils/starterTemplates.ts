import type { TemplateExample } from "api/typesGenerated";

export type StarterTemplatesByTag = Record<string, TemplateExample[]>;

export const getTemplatesByTag = (
  templates: TemplateExample[],
): StarterTemplatesByTag => {
  const tags: StarterTemplatesByTag = {
    all: templates,
  };

  templates.forEach((template) => {
    template.tags.forEach((tag) => {
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- this can be undefined
      if (tags[tag]) {
        tags[tag].push(template);
      } else {
        tags[tag] = [template];
      }
    });
  });

  return tags;
};
