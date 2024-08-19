import { forLightThemes } from "../externalImages";
import experimental from "./experimental";
import monaco from "./monaco";
import muiTheme from "./mui";
import permission from "./permission";
import roles from "./roles";

export default {
	...muiTheme,
	externalImages: forLightThemes,
	experimental,
	monaco,
	roles,
	permission,
};
