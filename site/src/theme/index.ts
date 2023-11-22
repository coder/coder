import { dark } from "./mui";
import { dark as experimental } from "./experimental";

export type Theme = typeof theme;

const theme = {
  ...dark,
  experimental,
};

export default theme;
