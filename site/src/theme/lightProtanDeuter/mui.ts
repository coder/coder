/**
 * @deprecated MUI theme is deprecated. Migrate to Tailwind CSS theme system.
 *
 * The colorblind-friendly palette is expressed through `roles.ts` and the
 * CSS variables in `site/src/index.css`. We re-export the base light MUI
 * theme so `palette.mode === "light"` stays correct for any remaining
 * legacy MUI component that inspects it, including the Shiki theme
 * selector in `DiffViewer.tsx`.
 */
export { default } from "../light/mui";
