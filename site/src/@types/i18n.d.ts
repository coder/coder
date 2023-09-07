import "i18next";

// https://github.com/i18next/react-i18next/issues/1543#issuecomment-1528679591
declare module "i18next" {
  interface TypeOptions {
    returnNull: false;
    allowObjectInHTMLChildren: false;
  }
  export function t<T>(s: string): T;
}
