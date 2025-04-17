import { execSync } from "node:child_process";
import * as path from "node:path";

export default function () {
	// If running terraform tests, verify the requirements exist in the
	// environment.
	//
	// These execs will throw an error if the status code is non-zero.
	// So if both these work, then we can launch terraform provisioners.
	let hasTerraform = false;
	let hasDocker = false;
	try {
		execSync("terraform --version");
		hasTerraform = true;
	} catch {
		/* empty */
	}

	try {
		execSync("docker --version");
		hasDocker = true;
	} catch {
		/* empty */
	}

	if (!hasTerraform || !hasDocker) {
		const msg = `Terraform provisioners require docker & terraform binaries to function. \n${
			hasTerraform
				? ""
				: "\tThe `terraform` executable is not present in the runtime environment.\n"
		}${
			hasDocker
				? ""
				: "\tThe `docker` executable is not present in the runtime environment.\n"
		}`;
		throw new Error(msg);
	}

	if (!process.env.CI && !process.env.CODER_E2E_REUSE_EXISTING_SERVER) {
		console.info("==> make site/e2e/bin/coder");
		execSync("make site/e2e/bin/coder", {
			cwd: path.join(__dirname, "../../../"),
		});
	}
}
