import type { Palette } from "../palette";
import tw from "../tailwindColors";

const palette: Palette = {
	mode: "dark",
	primary: {
		main: tw.sky[500],
	},
	secondary: {
		main: tw.zinc[500],
	},
	background: {
		default: tw.zinc[950],
		paper: tw.zinc[900],
	},
	text: {
		primary: tw.zinc[50],
	},
};

export default palette;
