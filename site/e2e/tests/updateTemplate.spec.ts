import { test } from "@playwright/test";
import { createTemplate, updateTemplateSettings } from "../helpers";

test.only("template update with new name redirects on successful submit", async ({
  page,
}) => {
  const templateName = await createTemplate(page);

  await updateTemplateSettings(page, templateName, {
    name: "new-name",
  });
});
