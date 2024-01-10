import experimental from "./experimental";
import monaco from "./monaco";
import muiTheme from "./mui";
import { forDarkThemes } from "../externalImages";

export default {
  ...muiTheme,
  experimental,
  monaco,
  externalImages: forDarkThemes,
};
