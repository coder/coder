import { docs } from "#/utils/docs";

// Keep in sync with cli/login.go. The values are forwarded to the Coder licensor,
// so changing them requires coordinating with the licensor service.
export const numberOfDevelopersOptions = [
	"1 - 50",
	"51 - 100",
	"101 - 200",
	"201 - 500",
	"501 - 1000",
	"1001 - 2500",
	"2500+",
];

export const DATABASE_DOCS_LINK = docs(
	"/admin/infrastructure/architecture#postgresql-recommended",
);

export const CODER_PRIVACY_POLICY_LINK =
	"https://coder.com/legal/privacy-policy";
