import { forDarkThemes } from "../externalImages";
import experimental from "./experimental";
import monaco from "./monaco";
import muiTheme from "./mui";
import colorRoles from "./colorRoles";

export default {
	...muiTheme,
	externalImages: forDarkThemes,
	experimental,
	monaco,
	colorRoles,
};
