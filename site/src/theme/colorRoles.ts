export type ThemeRole = keyof ColorRoles;

export type InteractiveThemeRole = keyof {
	[K in keyof ColorRoles as ColorRoles[K] extends InteractiveColorRole
		? K
		: never]: unknown;
};

export interface ColorRoles {
	/** The default color role for general use */
	default: ColorRole;

	/** Something is wrong; either unexpectedly, or in a meaningful way. */
	error: ColorRole;

	/** Something isn't quite right, but without serious consequence. */
	warning: ColorRole;

	/** A prompt for action, to correct or look into something. */
	notice: ColorRole;

	/** Notable information; just so you know! */
	info: ColorRole;

	/** Confirmation, or affirming that things are as desired. */
	success: InteractiveColorRole;

	/** Selected, in progress, of particular relevance right now. */
	active: InteractiveColorRole;

	/** For things that can be made "active", but are not currently so.
	 * Paused, stopped, off, etc.
	 */
	inactive: ColorRole;

	/** Actions that have long lasting or irreversible effects.
	 * Deletion, immutable parameters, etc.
	 */
	danger: InteractiveColorRole;

	/** This isn't quite ready for prime-time, but you're welcome to look around!
	 * Preview features, experiments, unstable, etc.
	 */
	preview: ColorRole;
}

/**
 * A set of colors which work together to fill a desirable "communication role"
 * ie. I wish to communicate an error, I wish to communicate that this is dangerous, etc.
 */
export interface ColorRole {
	/** A background color that works best with the corresponding `outline` and `text` colors */
	background: string;

	/** A border, or a color for an outlined icon */
	outline: string;

	/** A color for text on the corresponding `background` */
	text: string;

	/** A set of more saturated colors to make things stand out */
	fill: {
		/** A saturated color for use as a background, or icons on a neutral background */
		solid: string;

		/** A color for outlining an area using the solid background color, or for text or for an outlined icon */
		outline: string;

		/** A color for text when using the `solid` background color */
		text: string;
	};
}

/** Provides additional colors which can indicate different states for interactive elements */
export interface InteractiveColorRole extends ColorRole {
	/** A set of colors which can indicate a disabled state */
	disabled: ColorRole;

	/** A set of colors which can indicate mouse hover (or keyboard focus)  */
	hover: ColorRole;
}
