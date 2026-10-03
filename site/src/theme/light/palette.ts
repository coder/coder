import type { Palette } from "../palette";
import tw from "../tailwindColors";

const palette: Palette = {
	mode: "light",
	primary: {
		main: tw.sky[600],
	},
	secondary: {
		main: tw.zinc[500],
	},
	background: {
		default: tw.zinc[50],
		paper: tw.zinc[100],
	},
	text: {
		primary: tw.zinc[950],
	},
};

export default palette;
