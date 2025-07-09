export type ThemeRole = keyof Roles;

type InteractiveThemeRole = keyof {
	[K in keyof Roles as Roles[K] extends InteractiveRole ? K : never]: unknown;
};

export interface Roles {
	/** Something is wrong; either unexpectedly, or in a meaningful way. */
	error: Role;

	/** Something isn't quite right, but without serious consequence. */
	warning: Role;

	/** A prompt for action, to correct or look into something. */
	notice: Role;

	/** Notable information; just so you know! */
	info: Role;

	/** Confirmation, or affirming that things are as desired. */
	success: InteractiveRole;

	/** Selected, in progress, of particular relevance right now. */
	active: InteractiveRole;

	/** For things that can be made "active", but are not currently so.
	 * Paused, stopped, off, etc.
	 */
	inactive: Role;

	/** Actions that have long lasting or irreversible effects.
	 * Deletion, immutable parameters, etc.
	 */
	danger: InteractiveRole;

	/** This isn't quite ready for prime-time, but you're welcome to look around!
	 * Preview features, experiments, unstable, etc.
	 */
	preview: Role;
}

/**
 * A set of colors which work together to fill a desirable "communication role"
 * ie. I wish to communicate an error, I wish to communicate that this is dangerous, etc.
 */
export interface Role {
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
export interface InteractiveRole extends Role {
	/** A set of colors which can indicate a disabled state */
	disabled: Role;

	/** A set of colors which can indicate mouse hover (or keyboard focus)  */
	hover: Role;
}
