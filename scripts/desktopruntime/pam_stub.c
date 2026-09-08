/*
 * PAM stub for the Coder desktop runtime build.
 *
 * TigerVNC's CMake build requires PAM development files even when the server
 * never performs PAM authentication. Xvnc in this runtime is started with
 * "-SecurityTypes None" on localhost and the Coder agent performs
 * authentication, so the PAM code paths (rfb::UnixPasswordValidator, used only
 * by the Plain and RSA-AES security types) are unreachable. Linking a real
 * libpam would also make a fully static binary impossible because PAM loads
 * authentication modules with dlopen at runtime.
 *
 * These stubs fail closed: every entry point reports an authentication error.
 */

#include <security/pam_appl.h>
#include <stddef.h>

int pam_start(const char *service, const char *user,
	const struct pam_conv *conv, pam_handle_t **pamh)
{
	(void)service;
	(void)user;
	(void)conv;
	if (pamh != NULL) {
		*pamh = NULL;
	}
	return PAM_ABORT;
}

int pam_end(pam_handle_t *pamh, int status)
{
	(void)pamh;
	(void)status;
	return PAM_ABORT;
}

int pam_authenticate(pam_handle_t *pamh, int flags)
{
	(void)pamh;
	(void)flags;
	return PAM_AUTH_ERR;
}

int pam_acct_mgmt(pam_handle_t *pamh, int flags)
{
	(void)pamh;
	(void)flags;
	return PAM_AUTH_ERR;
}

int pam_set_item(pam_handle_t *pamh, int item_type, const void *item)
{
	(void)pamh;
	(void)item_type;
	(void)item;
	return PAM_ABORT;
}

int pam_get_item(const pam_handle_t *pamh, int item_type, const void **item)
{
	(void)pamh;
	(void)item_type;
	if (item != NULL) {
		*item = NULL;
	}
	return PAM_ABORT;
}

int pam_open_session(pam_handle_t *pamh, int flags)
{
	(void)pamh;
	(void)flags;
	return PAM_ABORT;
}

int pam_close_session(pam_handle_t *pamh, int flags)
{
	(void)pamh;
	(void)flags;
	return PAM_ABORT;
}

int pam_setcred(pam_handle_t *pamh, int flags)
{
	(void)pamh;
	(void)flags;
	return PAM_ABORT;
}

int pam_putenv(pam_handle_t *pamh, const char *name_value)
{
	(void)pamh;
	(void)name_value;
	return PAM_ABORT;
}

char **pam_getenvlist(pam_handle_t *pamh)
{
	(void)pamh;
	return NULL;
}

const char *pam_strerror(pam_handle_t *pamh, int errnum)
{
	(void)pamh;
	(void)errnum;
	return "PAM support is not built into this runtime";
}
