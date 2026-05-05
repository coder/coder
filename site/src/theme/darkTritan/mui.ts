/**
 * @deprecated MUI theme is deprecated. Migrate to Tailwind CSS theme system.
 *
 * The colorblind-friendly palette is expressed through `roles.ts` and the
 * CSS variables in `site/src/index.css`. We re-export the base dark MUI
 * theme so `palette.mode === "dark"` stays correct for any remaining
 * legacy MUI component that inspects it.
 */
export { default } from "../dark/mui";
