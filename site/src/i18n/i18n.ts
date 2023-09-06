import i18next from "i18next";
import { initReactI18next } from "react-i18next";
import { en } from "./en";

export const defaultNS = "common";
export const resources = { en } as const;

export const i18n = i18next.use(initReactI18next);

i18n
  .init({
    fallbackLng: "en",
    interpolation: {
      escapeValue: false, // not needed for react as it escapes by default
    },
    resources,
  })
  .catch((error) => {
    // we are catching here to avoid lint's no-floating-promises error
    console.error("[Translation Service]:", error);
  });
