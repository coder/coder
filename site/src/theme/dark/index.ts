import { forDarkThemes } from "../externalImages";
import experimental from "./experimental";
import monaco from "./monaco";
import muiTheme from "./mui";
import roles from "./roles";
import branding from "./branding";

export default {
	...muiTheme,
	externalImages: forDarkThemes,
	experimental,
	branding,
	monaco,
	roles,
};
