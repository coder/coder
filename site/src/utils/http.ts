import type { ThemeRole } from "theme/roles";

export const httpStatusColor = (httpStatus: number): ThemeRole => {
	// Treat server errors (500) as errors
	if (httpStatus >= 500) {
		return "error";
	}

	// Treat client errors (400) as warnings
	if (httpStatus >= 400) {
		return "warning";
	}

	// OK (200) and redirects (300) are successful
	return "success";
};
