import { Maybe } from "components/Conditionals/Maybe";
import { useTranslation } from "react-i18next";

export const TTLHelperText = ({
  ttl,
  translationName,
}: {
  ttl?: number;
  translationName: string;
}) => {
  const { t } = useTranslation("templateSettingsPage");
  const count = typeof ttl !== "number" ? 0 : ttl;
  return (
    // no helper text if ttl is negative - error will show once field is considered touched
    <Maybe condition={count >= 0}>
      <span>{t(translationName, { count })}</span>
    </Maybe>
  );
};
