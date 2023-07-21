#include "pam-utils.h"

#include <security/pam_appl.h>
#include <assert.h>

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

static struct pam_response *
send_msg_generic (pam_handle_t *pamh, const struct pam_message *pam_msg)
{
  const struct pam_conv *pc;
  struct pam_response *resp;

  if (pam_get_item (pamh, PAM_CONV, (const void **) &pc) != PAM_SUCCESS)
    return NULL;

  if (!pc || !pc->conv)
    return NULL;

  if (pc->conv (1, (const struct pam_message *[]){ pam_msg }, &resp,
                pc->appdata_ptr) != PAM_SUCCESS)
    return NULL;

  return resp;
}

struct pam_response *
send_msg (pam_handle_t *pamh, const char *msg, int style)
{
  const struct pam_message pam_msg = {
    .msg_style = style,
    .msg = msg,
  };

  return send_msg_generic (pamh, &pam_msg);
}

GdmPamExtensionChoiceListRequest *
gdm_choices_request_create (const char *title,
                            size_t      num)
{
  GdmPamExtensionChoiceListRequest *request;

  request = calloc (1, GDM_PAM_EXTENSION_CHOICE_LIST_REQUEST_SIZE (num));
  GDM_PAM_EXTENSION_CHOICE_LIST_REQUEST_INIT (request, (char *) title, num);

  return request;
}

void
gdm_choices_request_set (GdmPamExtensionChoiceListRequest *request, size_t i,
                         const char *key, const char *text)
{
  assert (request != NULL && i < request->list.number_of_items);

  request->list.items[i].key = strdup (key);
  request->list.items[i].text = strdup (text);
}

void
gdm_choices_request_free (GdmPamExtensionChoiceListRequest *request)
{
  assert (request != NULL);

  for (size_t i = 0; i < request->list.number_of_items; ++i)
    {
      free ((char *) request->list.items[i].key);
      free ((char *) request->list.items[i].text);
    }

  free (request);
}

char *
gdm_choices_request_ask (pam_handle_t *pamh, GdmPamExtensionChoiceListRequest *request)
{
  GdmPamExtensionChoiceListResponse *response;
  struct pam_message prompt_message;
  struct pam_response *reply;
  char *ret_key;

  GDM_PAM_EXTENSION_MESSAGE_TO_BINARY_PROMPT_MESSAGE (request, &prompt_message);
  reply = send_msg_generic (pamh, &prompt_message);

  if (!reply)
    return NULL;

  response = GDM_PAM_EXTENSION_REPLY_TO_CHOICE_LIST_RESPONSE (reply);
  ret_key = strdup (response->key);
  free (response);

  return ret_key;
}
