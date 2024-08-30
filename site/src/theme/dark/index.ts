import { forDarkThemes } from "../externalImages";
import roles from "./colorRoles";
import experimental from "./experimental";
import monaco from "./monaco";
import muiTheme from "./mui";

export default {
	...muiTheme,
	externalImages: forDarkThemes,
	experimental,
	monaco,
	roles,
};
