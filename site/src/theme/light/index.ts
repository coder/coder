import { forLightThemes } from "../externalImages";
import roles from "./colorRoles";
import experimental from "./experimental";
import monaco from "./monaco";
import muiTheme from "./mui";

export default {
	...muiTheme,
	externalImages: forLightThemes,
	experimental,
	monaco,
	roles,
};
