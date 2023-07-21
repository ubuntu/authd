#include "pam-utils.h"

#include <security/pam_appl.h>

char *
argv_string_get (const char **argv, unsigned i)
{
  return strdup (argv[i]);
}

char *
get_user (pam_handle_t *pamh, const char *prompt)
{
  if (!pamh)
    return NULL;
  int pam_err = 0;
  const char *user;

  if ((pam_err = pam_get_user (pamh, &user, prompt)) != PAM_SUCCESS)
    return NULL;
  return strdup (user);
}

const char *
get_module_name (pam_handle_t *pamh)
{
  const char *module_name;

  if (pam_get_item (pamh, PAM_SERVICE, (const void **) &module_name) != PAM_SUCCESS)
    return NULL;

  return module_name;
}

struct pam_response *
send_msg (pam_handle_t *pamh, const char *msg, int style)
{
  const struct pam_message pam_msg = {
    .msg_style = style,
    .msg = msg,
  };
  const struct pam_conv *pc;
  struct pam_response *resp;

  if (pam_get_item (pamh, PAM_CONV, (const void **) &pc) != PAM_SUCCESS)
    return NULL;

  if (!pc || !pc->conv)
    return NULL;

  if (pc->conv (1, (const struct pam_message *[]){ &pam_msg }, &resp,
                pc->appdata_ptr) != PAM_SUCCESS)
    return NULL;

  return resp;
}
